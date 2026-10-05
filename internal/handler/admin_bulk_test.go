package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// bulkProbe is a runner that records each item it is asked to run and
// answers from a table of canned answers by path.
type bulkProbe struct {
	mu      sync.Mutex
	calls   []BulkCall
	answers map[string]BulkAnswer
	// before runs ahead of each call.
	before func(BulkCall)
}

func (p *bulkProbe) run(_ context.Context, _ *gin.Context, call BulkCall) BulkAnswer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.before != nil {
		p.before(call)
	}
	p.calls = append(p.calls, call)
	if answer, ok := p.answers[call.Path]; ok {
		return answer
	}
	return BulkAnswer{Status: http.StatusOK, Body: []byte(`{"code":0,"data":{"message":"ok"},"msg":"操作成功","ts":1}`)}
}

func panelRefusal(message string) BulkAnswer {
	return BulkAnswer{Status: http.StatusOK, Body: []byte(fmt.Sprintf(`{"code":-1,"data":null,"msg":%q,"ts":1}`, message))}
}

type bulkReply struct {
	Data BulkResult `json:"data"`
}

// bulkRouter serves the handler as administrator 7, recording the audit
// action the handler names.
func bulkRouter(probe *bulkProbe, audited *string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("email", "admin@example.test")
		c.Next()
		if audited != nil {
			if action, ok := c.Get(middleware.AuditActionKey); ok {
				*audited = action.(string)
			}
		}
	})
	handler := NewAdminBulkHandler(probe.run)
	router.POST("/api/v4/admin/users/bulk", handler.Users)
	router.POST("/api/v4/admin/invite-codes/bulk", handler.InviteCodes)
	return router
}

func bulkPost(router *gin.Engine, path, body string, headers ...string) (*httptest.ResponseRecorder, bulkReply) {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var reply bulkReply
	_ = json.Unmarshal(recorder.Body.Bytes(), &reply)
	return recorder, reply
}

// Each id is one run of the single-item route, in the order asked, repeats
// dropped, with its own Idempotency-Key and request id; the answer lists every
// item, and the audit row is named after the action.
func TestBulkUsersRunsTheSingleItemRoutePerID(t *testing.T) {
	probe := &bulkProbe{}
	var audited string
	recorder, reply := bulkPost(bulkRouter(probe, &audited), "/api/v4/admin/users/bulk", `{"action":"ban","ids":[12,5,12,9]}`, "Idempotency-Key", "k-1")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "bulk_ban", audited)
	require.Equal(t, BulkResult{
		Action: "ban", Requested: 3, Succeeded: 3, Failed: 0,
		Results: []BulkItemResult{{ID: 12, OK: true}, {ID: 5, OK: true}, {ID: 9, OK: true}},
	}, reply.Data)
	require.Len(t, probe.calls, 3)
	for index, id := range []string{"12", "5", "9"} {
		call := probe.calls[index]
		require.Equal(t, http.MethodPost, call.Method)
		require.Equal(t, "/api/v2/admin/users/"+id+"/ban", call.Path)
		require.Equal(t, gin.Params{{Key: "id", Value: id}}, call.Params)
		require.Equal(t, "k-1:"+id, call.IdempotencyKey, "a retried bulk request repeats each item's key")
	}
	require.Len(t, map[string]bool{probe.calls[0].RequestID: true, probe.calls[1].RequestID: true, probe.calls[2].RequestID: true}, 3)

	// The same request again sends the same keys.
	again := &bulkProbe{}
	_, _ = bulkPost(bulkRouter(again, nil), "/api/v4/admin/users/bulk", `{"action":"ban","ids":[12,5,9]}`, "Idempotency-Key", "k-1")
	require.Equal(t, []string{probe.calls[0].IdempotencyKey, probe.calls[1].IdempotencyKey, probe.calls[2].IdempotencyKey},
		[]string{again.calls[0].IdempotencyKey, again.calls[1].IdempotencyKey, again.calls[2].IdempotencyKey})
}

func TestBulkUsersActionsMapToTheirRoutes(t *testing.T) {
	for action, path := range map[string]string{
		"ban": "/api/v2/admin/users/3/ban", "unban": "/api/v2/admin/users/3/unban", "reset_traffic": "/api/v2/admin/users/3/reset-traffic",
	} {
		probe := &bulkProbe{}
		_, reply := bulkPost(bulkRouter(probe, nil), "/api/v4/admin/users/bulk", `{"action":"`+action+`","ids":[3]}`)
		require.Equal(t, 1, reply.Data.Succeeded, action)
		require.Equal(t, path, probe.calls[0].Path, action)
		require.Equal(t, http.MethodPost, probe.calls[0].Method)
	}
	probe := &bulkProbe{}
	_, reply := bulkPost(bulkRouter(probe, nil), "/api/v4/admin/invite-codes/bulk", `{"action":"revoke","ids":[4]}`)
	require.Equal(t, 1, reply.Data.Succeeded)
	require.Equal(t, "/api/v2/admin/invite/codes/4", probe.calls[0].Path)
	require.Equal(t, http.MethodDelete, probe.calls[0].Method)
}

// Items are independent: a refusal does not stop the others, and every
// refusal carries a code and the single-item route's own message.
func TestBulkItemsFailIndependently(t *testing.T) {
	probe := &bulkProbe{answers: map[string]BulkAnswer{
		"/api/v2/admin/users/2/ban": panelRefusal("用户不存在"),
		"/api/v2/admin/users/3/ban": panelRefusal("封禁失败: database is locked"),
		"/api/v2/admin/users/4/ban": {Status: http.StatusServiceUnavailable, Body: []byte(`{"error":{"code":"package_route_frozen","message":"package route is paused for maintenance"}}`)},
		"/api/v2/admin/users/5/ban": {Status: http.StatusInternalServerError},
		"/api/v2/admin/users/6/ban": {Status: http.StatusBadGateway, Body: []byte(`{"message":"bridge down"}`)},
	}}
	recorder, reply := bulkPost(bulkRouter(probe, nil), "/api/v4/admin/users/bulk", `{"action":"ban","ids":[1,2,3,4,5,6,8]}`)
	require.Equal(t, http.StatusOK, recorder.Code, "per-item failures are not an HTTP failure")
	require.Equal(t, 7, reply.Data.Requested)
	require.Equal(t, 2, reply.Data.Succeeded)
	require.Equal(t, 5, reply.Data.Failed)
	require.Len(t, probe.calls, 7, "a failed item does not stop the batch")
	byID := map[uint]BulkItemResult{}
	for _, result := range reply.Data.Results {
		byID[result.ID] = result
	}
	require.True(t, byID[1].OK)
	require.Nil(t, byID[1].Error)
	require.True(t, byID[8].OK)
	require.Equal(t, &BulkError{Code: "not_found", Message: "用户不存在"}, byID[2].Error)
	require.Equal(t, &BulkError{Code: "failed", Message: "封禁失败: database is locked"}, byID[3].Error)
	require.Equal(t, &BulkError{Code: "package_route_frozen", Message: "package route is paused for maintenance"}, byID[4].Error)
	require.Equal(t, "failed", byID[5].Error.Code)
	require.NotEmpty(t, byID[5].Error.Message)
	require.Equal(t, &BulkError{Code: "failed", Message: "bridge down"}, byID[6].Error)
	require.False(t, byID[2].OK)
}

func TestBulkInviteCodeRefusals(t *testing.T) {
	probe := &bulkProbe{answers: map[string]BulkAnswer{
		"/api/v2/admin/invite/codes/1": panelRefusal("invite code not found"),
		"/api/v2/admin/invite/codes/2": panelRefusal("invite code already used"),
	}}
	_, reply := bulkPost(bulkRouter(probe, nil), "/api/v4/admin/invite-codes/bulk", `{"action":"revoke","ids":[1,2,3]}`)
	require.Equal(t, "not_found", reply.Data.Results[0].Error.Code)
	require.Equal(t, "conflict", reply.Data.Results[1].Error.Code)
	require.True(t, reply.Data.Results[2].OK)
}

// An administrator cannot ban themselves through a bulk request (a page
// selected whole holds them); the route does not run for that item, and
// unbanning or resetting themselves is allowed.
func TestBulkUsersRefuseToBanTheCaller(t *testing.T) {
	probe := &bulkProbe{}
	_, reply := bulkPost(bulkRouter(probe, nil), "/api/v4/admin/users/bulk", `{"action":"ban","ids":[6,7,8]}`)
	require.Equal(t, 2, reply.Data.Succeeded)
	require.Equal(t, &BulkError{Code: "forbidden_self", Message: "an administrator cannot ban their own account"}, reply.Data.Results[1].Error)
	require.Len(t, probe.calls, 2)
	for _, call := range probe.calls {
		require.NotContains(t, call.Path, "/7/")
	}

	probe = &bulkProbe{}
	_, reply = bulkPost(bulkRouter(probe, nil), "/api/v4/admin/users/bulk", `{"action":"unban","ids":[7]}`)
	require.Equal(t, 1, reply.Data.Succeeded)
}

// A request that cannot be read, names an action the resource lacks, or has no
// valid ids is refused whole, 400 invalid_request, and nothing runs.
func TestBulkRefusesInvalidRequests(t *testing.T) {
	many := make([]string, 201)
	for i := range many {
		many[i] = fmt.Sprint(i + 1)
	}
	for name, body := range map[string]string{
		"not JSON":               `ban`,
		"an array":               `[1,2]`,
		"an unknown field":       `{"action":"ban","ids":[1],"force":true}`,
		"no action":              `{"ids":[1]}`,
		"an unknown action":      `{"action":"delete","ids":[1]}`,
		"reset the subscription": `{"action":"reset_subscribe","ids":[1]}`,
		"revoke on users":        `{"action":"revoke","ids":[1]}`,
		"no ids":                 `{"action":"ban"}`,
		"empty ids":              `{"action":"ban","ids":[]}`,
		"id zero":                `{"action":"ban","ids":[0]}`,
		"a negative id":          `{"action":"ban","ids":[-1]}`,
		"a string id":            `{"action":"ban","ids":["1"]}`,
		"a fractional id":        `{"action":"ban","ids":[1.5]}`,
		"an id beyond 32 bits":   `{"action":"ban","ids":[4294967296]}`,
		"more than 200 ids":      `{"action":"ban","ids":[` + strings.Join(many, ",") + `]}`,
		"two objects":            `{"action":"ban","ids":[1]}{"action":"ban","ids":[2]}`,
		"an oversized body":      `{"action":"ban","ids":[1],"x":"` + strings.Repeat("a", maxBulkBody) + `"}`,
	} {
		probe := &bulkProbe{}
		recorder, _ := bulkPost(bulkRouter(probe, nil), "/api/v4/admin/users/bulk", body)
		require.Equal(t, http.StatusBadRequest, recorder.Code, name)
		require.Contains(t, recorder.Body.String(), "invalid_request", name)
		require.Empty(t, probe.calls, name)
	}
	// Exactly 200 ids is allowed, and repeats do not count against it.
	probe := &bulkProbe{}
	recorder, reply := bulkPost(bulkRouter(probe, nil), "/api/v4/admin/users/bulk", `{"action":"unban","ids":[`+strings.Join(many[:200], ",")+`,1,2,3]}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 200, reply.Data.Succeeded)
}

// Items the request has no time left for are reported as not attempted, and
// their routes do not run.
func TestBulkStopsWhenTheRequestEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	probe := &bulkProbe{before: func(call BulkCall) {
		if call.Path == "/api/v2/admin/users/2/ban" {
			cancel()
		}
	}}
	router := bulkRouter(probe, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v4/admin/users/bulk", strings.NewReader(`{"action":"ban","ids":[1,2,3,4]}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var reply bulkReply
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &reply))
	require.Equal(t, 2, reply.Data.Succeeded)
	require.Equal(t, 2, reply.Data.Failed)
	require.Equal(t, "not_attempted", reply.Data.Results[2].Error.Code)
	require.Equal(t, "not_attempted", reply.Data.Results[3].Error.Code)
	require.Len(t, probe.calls, 2)
}

func TestSafeBulkRequestID(t *testing.T) {
	require.Equal(t, "req-1.a_b", safeBulkRequestID("req-1.a_b"))
	for _, unsafe := range []string{"", "a b", "a:b", "a/b", "ü", strings.Repeat("a", 151)} {
		generated := safeBulkRequestID(unsafe)
		require.NotEqual(t, unsafe, generated)
		require.Regexp(t, `^[A-Za-z0-9._-]+$`, generated)
	}
}
