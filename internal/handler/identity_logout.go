package handler

import (
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/gin-gonic/gin"
)

// IdentityLogout ends the calling session: its token stops working at once,
// in every Control process, until it would have expired anyway. Tokens
// issued before session ids existed carry none and cannot be ended alone.
func IdentityLogout(c *gin.Context) {
	sessionID := c.GetString("session_id")
	if sessionID == "" {
		kernelData(c, http.StatusOK, gin.H{"logged_out": false, "reason": "the token has no session id"})
		return
	}
	expires, ok := c.Get("token_expires_at")
	expiresAt, _ := expires.(time.Time)
	if !ok || expiresAt.IsZero() {
		expiresAt = time.Now().Add(24 * time.Hour)
	}
	revocation := authn.Revocation{
		UserID: c.GetUint("user_id"), SessionID: sessionID, SessionExpiresAt: expiresAt, Reason: "logout",
	}
	var err error
	if store := authn.DefaultStore(); store != nil {
		err = store.Publish(c.Request.Context(), revocation)
	} else {
		err = authn.Write(database.GetDB().WithContext(c.Request.Context()), revocation)
	}
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "logout_failed", err.Error())
		return
	}
	kernelData(c, http.StatusOK, gin.H{"logged_out": true})
}
