package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// apiTokenFailureLimit bounds the refused API tokens one client address may
// present: the tenth refusal in five minutes locks the address out of token
// authentication for ten minutes. JWT sessions are not affected.
var apiTokenFailureLimit = service.LoginRateLimitOptions{
	Enabled:      true,
	MaxAttempts:  10,
	Window:       5 * time.Minute,
	Lockout:      10 * time.Minute,
	CleanupAfter: 20 * time.Minute,
}

// tokenServiceCache keeps one adminapitoken.Service per database handle, so
// its write throttles survive from request to request.
type tokenServiceCache struct {
	// source returns the database; nil means the process database.
	source func() *gorm.DB

	mu  sync.Mutex
	db  *gorm.DB
	svc *adminapitoken.Service
}

func (c *tokenServiceCache) get() *adminapitoken.Service {
	source := c.source
	if source == nil {
		source = database.GetDB
	}
	db := source()
	if db == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.svc == nil || c.db != db {
		c.db, c.svc = db, adminapitoken.New(db)
	}
	return c.svc
}

// AdminAPIToken authenticates "Authorization: Bearer anixadm_..." on the
// administrator APIs, in front of JWTAuth (docs/reference/admin-api-tokens.md).
//
//   - Only the Authorization header carries a token. A value that looks like
//     one in the URL is refused, never used: URLs reach logs and proxies.
//   - Any other request goes on to JWTAuth unchanged. JWTAuth skips the JWT
//     check for a request this middleware authenticated, and only for it: the
//     marker is set on the server side, and user routes, which have no
//     AdminAPIToken, refuse a token like any other string that is no JWT.
//   - The owner's rights are read from the database on every request, so a
//     banned, demoted or deleted administrator's tokens stop at once.
//   - The token's scope is enforced here: a read token makes GET and HEAD
//     requests only, and never the reads that answer a secret in clear.
//     Managing tokens needs a signed-in session, whatever the scope.
//   - Refusals count against the client address (apiTokenFailureLimit) and
//     are audited, at most once a minute per token.
func AdminAPIToken() gin.HandlerFunc {
	return adminAPIToken(&tokenServiceCache{}, service.NewLoginRateLimiter())
}

func adminAPIToken(cache *tokenServiceCache, failures *service.LoginRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, values := range c.Request.URL.Query() {
			for _, value := range values {
				if adminapitoken.LooksLikeToken(value) {
					// The request is refused unread. The URL has reached the
					// access log by now, so the owner should revoke the token.
					if tokens := cache.get(); tokens != nil {
						tokens.RecordDenied(c.Request.Context(), adminapitoken.Event{
							IP: c.ClientIP(), Method: c.Request.Method, Path: c.Request.URL.Path, Reason: "token_in_url",
						})
					}
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"code":    "api_token_in_url",
						"message": "an API token is accepted in the Authorization header only, never in the URL; revoke the token that was sent here",
					})
					return
				}
			}
		}
		presented, ok := bearerAPIToken(c.GetHeader("Authorization"))
		if !ok {
			c.Next()
			return
		}

		ip := c.ClientIP()
		limiterKey := "admin_api_token|" + ip
		if blocked, wait := failures.Check(limiterKey, apiTokenFailureLimit); blocked {
			c.Header("Retry-After", strconv.Itoa(max(int(wait.Seconds()), 1)))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code": "api_token_rate_limited", "message": "too many refused API tokens from this address, try again later",
			})
			return
		}
		tokens := cache.get()
		if tokens == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"code": "api_token_unavailable", "message": "API tokens are unavailable",
			})
			return
		}

		path := c.Request.URL.Path
		principal, err := tokens.Authenticate(c.Request.Context(), presented, ip)
		if err != nil {
			var denial *adminapitoken.Denial
			if !errors.As(err, &denial) {
				slog.Error("admin api token lookup failed", slog.String("error_type", "storage"))
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"code": "api_token_unavailable", "message": "API tokens are unavailable",
				})
				return
			}
			failures.RecordFailure(limiterKey, apiTokenFailureLimit)
			tokens.RecordDenied(c.Request.Context(), adminapitoken.Event{
				TokenID: denial.TokenID, UserID: denial.UserID, IP: ip, Method: c.Request.Method, Path: path, Reason: denial.Detail,
			})
			// One answer for every refusal: the caller learns nothing about
			// which tokens exist or who owns them.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": "api_token_invalid", "message": "invalid or expired API token",
			})
			return
		}

		event := adminapitoken.Event{
			TokenID: principal.TokenID, UserID: principal.UserID, Actor: principal.Email, IP: ip, Method: c.Request.Method, Path: path,
		}
		if denied := tokenRequestDenied(c, principal.Scope); denied != "" {
			event.Reason = denied
			tokens.RecordDenied(c.Request.Context(), event)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "api_token_forbidden", "message": "this API token may not make this request",
			})
			return
		}

		c.Set("user_id", principal.UserID)
		c.Set("email", principal.Email)
		c.Set("is_admin", true)
		c.Set(adminapitoken.ContextKeyAuthMethod, adminapitoken.AuthMethodAPIToken)
		c.Set(adminapitoken.ContextKeyTokenID, principal.TokenID)
		c.Set(adminapitoken.ContextKeyTokenScope, principal.Scope)
		c.Next()

		if !safeMethod(c.Request.Method) {
			event.Status = c.Writer.Status()
			tokens.RecordUse(context.WithoutCancel(c.Request.Context()), event)
		}
	}
}

// bearerAPIToken returns the token of an Authorization header that is a
// Bearer credential starting with the token prefix. Anything else, a JWT
// included, is not ours.
func bearerAPIToken(header string) (string, bool) {
	scheme, value, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	value = strings.TrimSpace(value)
	if !adminapitoken.LooksLikeToken(value) {
		return "", false
	}
	return value, true
}

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// tokenRequestDenied says why the scope refuses the request, or "" when it
// allows it.
func tokenRequestDenied(c *gin.Context, scope string) string {
	path := c.Request.URL.Path
	if strings.HasPrefix(path, adminapitoken.ManagementPath) {
		return "interactive_only"
	}
	switch scope {
	case model.AdminAPITokenScopeAdmin:
		return ""
	case model.AdminAPITokenScopeRead:
		switch {
		case !safeMethod(c.Request.Method):
			return "scope_read_only"
		case revealsSecret(c):
			return "scope_secret_read"
		}
		return ""
	}
	return "unknown_scope"
}

// secretReadRoutes are the administrator reads that still answer a secret in
// clear (the other secrets are masked in every answer): a proxy node's API
// key and shared secret, and the Telegram bot's token. A read scope token
// may not make them. A new route of this kind is added here.
var secretReadRoutes = map[string]bool{
	"/api/v2/admin/nodes/:id/credentials": true,
	"/api/v2/admin/telegram/bot":          true,
}

// revealsSecret reports whether a read answers a secret in clear: a route
// of secretReadRoutes (matched on the route, so a URL spelled differently
// is the same route), or a read the audit log records as "reveal".
func revealsSecret(c *gin.Context) bool {
	return secretReadRoutes[c.FullPath()] || auditedRead(c.Request.Method, c.Request.URL.Path)
}
