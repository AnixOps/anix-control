package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// bulkBudget bounds one bulk request, under the 30 seconds production
// configures as the server's write timeout, so the answer still reaches the
// client. An item takes milliseconds (the gateway allows one 30 seconds); the
// items the budget leaves no time for are reported as not attempted.
const bulkBudget = 25 * time.Second

// maxBulkIDs is the most ids one bulk request names.
const maxBulkIDs = 200

// maxBulkBody bounds a bulk request body: 200 ids are about 2 KiB.
const maxBulkBody = 64 << 10

// bulkAction is one action of a bulk resource: the single-item /api/v2 route
// it repeats, one id at a time, and how that route's refusals read.
type bulkAction struct {
	method string
	// path is the single-item route with {id} for the item.
	path string
	// refusals map the single-item route's error messages to codes; any other
	// message is code "failed".
	refusals map[string]string
	// guard refuses an item before the route runs: nil lets it run.
	guard func(actor, id uint) *BulkError
}

// bulkResource is what a bulk endpoint acts on: its actions by name.
type bulkResource struct {
	actions map[string]bulkAction
}

// bulkJob is a validated bulk request.
type bulkJob struct {
	Action string
	IDs    []uint
}

var userBulkResource = bulkResource{actions: map[string]bulkAction{
	"ban": {
		method: http.MethodPost, path: "/api/v2/admin/users/{id}/ban",
		refusals: map[string]string{service.ErrUserNotFound.Error(): "not_found"},
		// A page selected whole includes the administrator: a bulk ban must
		// not lock them out of the console.
		guard: func(actor, id uint) *BulkError {
			if actor != 0 && actor == id {
				return &BulkError{Code: "forbidden_self", Message: "an administrator cannot ban their own account"}
			}
			return nil
		},
	},
	"unban":         {method: http.MethodPost, path: "/api/v2/admin/users/{id}/unban", refusals: map[string]string{service.ErrUserNotFound.Error(): "not_found"}},
	"reset_traffic": {method: http.MethodPost, path: "/api/v2/admin/users/{id}/reset-traffic", refusals: map[string]string{service.ErrUserNotFound.Error(): "not_found"}},
}}

var inviteCodeBulkResource = bulkResource{actions: map[string]bulkAction{
	"revoke": {
		method: http.MethodDelete, path: "/api/v2/admin/invite/codes/{id}",
		refusals: map[string]string{
			service.ErrInviteCodeNotFound.Error(): "not_found",
			service.ErrInviteCodeUsed.Error():     "conflict",
		},
	},
}}

// BulkError is why one item of a bulk request was not done.
type BulkError struct {
	// Code is machine readable: not_found, conflict, forbidden_self, failed,
	// not_attempted, or a code of the package gateway (package_route_frozen,
	// plugin_host_unavailable, ...).
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BulkItemResult is the outcome of one id, in the order of the request.
type BulkItemResult struct {
	ID    uint       `json:"id"`
	OK    bool       `json:"ok"`
	Error *BulkError `json:"error,omitempty"`
}

// BulkResult is the answer of a bulk request.
type BulkResult struct {
	Action    string           `json:"action"`
	Requested int              `json:"requested"`
	Succeeded int              `json:"succeeded"`
	Failed    int              `json:"failed"`
	Results   []BulkItemResult `json:"results"`
}

// BulkCall is one single-item request of a bulk request.
type BulkCall struct {
	Method string
	Path   string
	Params gin.Params
	// IdempotencyKey makes the single-item request apply once when the bulk
	// request is retried.
	IdempotencyKey string
	// RequestID identifies the single-item request in logs and ledgers.
	RequestID string
}

// BulkAnswer is what the single-item route answered.
type BulkAnswer struct {
	Status int
	Body   []byte
}

// BulkRunner runs one single-item request as the administrator who sent the
// bulk request (parent), within ctx, and answers what the route answered. The
// router's runner (GatewayBulkRunner) goes through the /api/v2 package
// gateway, so a route runs wherever its mode puts it: in the identity
// package or in the kernel's legacy handler.
type BulkRunner func(ctx context.Context, parent *gin.Context, call BulkCall) BulkAnswer

// AdminBulkHandler serves the bulk endpoints of the console's tables.
type AdminBulkHandler struct {
	run BulkRunner
}

// NewAdminBulkHandler returns the handler with the runner that performs each
// item.
func NewAdminBulkHandler(run BulkRunner) *AdminBulkHandler {
	return &AdminBulkHandler{run: run}
}

type bulkRequest struct {
	Action string   `json:"action"`
	IDs    []uint64 `json:"ids"`
}

// Users godoc
// @Summary 批量操作用户
// @Description 对 ids 中的每个用户逐个执行单用户接口 (封禁 POST /admin/users/{id}/ban, 解封 .../unban, 重置流量 .../reset-traffic), 经同一个 /api/v2 网关, 所以与单个请求走同一套处理和路由模式。ids 最多 200 个, 去重后按请求顺序处理。不是全有或全无: 每个 id 独立生效、独立失败, 总是 HTTP 200 并返回逐个结果 {id, ok, error{code, message}} (请求本身无效才是 400)。封禁与解封可重复执行 (已处于目标状态也算成功); 重置流量在请求带 Idempotency-Key 时重试只生效一次。管理员不能在批量中封禁自己 (forbidden_self)。请求按 bulk_<action> 记入审计日志, 每个 id 另记一条与单个请求相同的审计记录。不提供删除用户与重置订阅链接。
// @Tags 管理端-用户
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param Idempotency-Key header string false "重试同一批量请求时使用同一个键"
// @Param request body object true "{action: ban|unban|reset_traffic, ids: [用户ID...]}"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/users/bulk [post]
func (h *AdminBulkHandler) Users(c *gin.Context) {
	h.serve(c, userBulkResource)
}

// InviteCodes godoc
// @Summary 批量操作邀请码
// @Description 对 ids 中的每个邀请码逐个执行单个接口 (撤销 DELETE /admin/invite/codes/{id}), 语义同批量用户操作: ids 最多 200 个, 每个 id 独立生效, 总是 HTTP 200 并返回逐个结果。已使用的邀请码保留并报告 conflict, 已撤销或不存在的报告 not_found (重试一次部分成功的批量时, 已撤销的会报 not_found)。
// @Tags admin-invite-codes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "{action: revoke, ids: [邀请码ID...]}"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/invite-codes/bulk [post]
func (h *AdminBulkHandler) InviteCodes(c *gin.Context) {
	h.serve(c, inviteCodeBulkResource)
}

// serve validates a bulk request, runs the action's single-item route for
// each id in turn and answers with every outcome.
func (h *AdminBulkHandler) serve(c *gin.Context, resource bulkResource) {
	request, err := readBulkRequest(c, resource)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	action := resource.actions[request.Action]
	c.Set(middleware.AuditActionKey, "bulk_"+request.Action)

	actor := actorUserID(c)
	token := assignmentToken(c)
	baseRequestID := safeBulkRequestID(c.GetString("request_id"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), bulkBudget)
	defer cancel()

	result := BulkResult{Action: request.Action, Requested: len(request.IDs), Results: make([]BulkItemResult, 0, len(request.IDs))}
	audit := make([]middleware.AuditItem, 0, len(request.IDs))
	for index, id := range request.IDs {
		item := BulkItemResult{ID: id}
		path := strings.Replace(action.path, "{id}", strconv.FormatUint(uint64(id), 10), 1)
		status := http.StatusOK
		if refusal := h.attempt(ctx, c, action, actor, id, path, token, fmt.Sprintf("%s.%d", baseRequestID, index+1)); refusal != nil {
			item.Error, status = refusal.error, refusal.status
			result.Failed++
		} else {
			item.OK = true
			result.Succeeded++
		}
		result.Results = append(result.Results, item)
		entry := middleware.AuditItem{Method: action.method, Path: path, Status: status}
		if item.Error != nil {
			entry.Error = item.Error.Code + ": " + item.Error.Message
		}
		audit = append(audit, entry)
	}
	middleware.RecordAuditItems(c, audit)
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, result)
}

// bulkRefusal is a failed item with the HTTP status its single-item request
// would have been audited under.
type bulkRefusal struct {
	error  *BulkError
	status int
}

// attempt runs one item. It returns nil when the item was done.
func (h *AdminBulkHandler) attempt(ctx context.Context, parent *gin.Context, action bulkAction, actor, id uint, path, token, requestID string) *bulkRefusal {
	if action.guard != nil {
		if refused := action.guard(actor, id); refused != nil {
			return &bulkRefusal{error: refused, status: http.StatusForbidden}
		}
	}
	if err := ctx.Err(); err != nil {
		return &bulkRefusal{error: &BulkError{Code: "not_attempted", Message: "the request ended before this item ran"}, status: http.StatusGatewayTimeout}
	}
	answer := h.run(ctx, parent, BulkCall{
		Method: action.method, Path: path,
		Params:         gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(id), 10)}},
		IdempotencyKey: token + ":" + strconv.FormatUint(uint64(id), 10),
		RequestID:      requestID,
	})
	return interpretBulkAnswer(action, answer)
}

// interpretBulkAnswer reads what a single-item route answered: a panel
// envelope with code 0 is done; a panel envelope with another code, a gateway
// error ({"error": {"code", "message"}}) and a {"message"} body are refusals.
func interpretBulkAnswer(action bulkAction, answer BulkAnswer) *bulkRefusal {
	var body struct {
		Code    *int   `json:"code"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(answer.Body, &body)
	switch {
	case answer.Status == http.StatusOK && body.Code != nil && *body.Code == 0:
		return nil
	case body.Error != nil && body.Error.Code != "":
		return &bulkRefusal{error: &BulkError{Code: body.Error.Code, Message: body.Error.Message}, status: statusOrDefault(answer.Status)}
	}
	message := body.Msg
	if message == "" {
		message = body.Message
	}
	if message == "" {
		message = "the request was refused"
		if answer.Status == 0 {
			message = "the request did not complete"
		}
	}
	code := "failed"
	if refusal, known := action.refusals[message]; known {
		code = refusal
	}
	refusal := &bulkRefusal{error: &BulkError{Code: code, Message: message}, status: statusOrDefault(answer.Status)}
	switch code {
	case "not_found":
		refusal.status = http.StatusNotFound
	case "conflict":
		refusal.status = http.StatusConflict
	}
	return refusal
}

// statusOrDefault is the status a refusal is audited under: the route's own
// when it failed at the HTTP level, else 422 (a panel envelope refusal is
// HTTP 200, so the single-item route's audit row says 200 for it).
func statusOrDefault(status int) int {
	if status >= http.StatusBadRequest {
		return status
	}
	return http.StatusUnprocessableEntity
}

// readBulkRequest parses and validates a bulk request body against the
// resource's actions: known fields only, a known action and 1 to maxBulkIDs
// distinct positive ids (repeats are dropped, the order kept).
func readBulkRequest(c *gin.Context, resource bulkResource) (bulkJob, error) {
	out := bulkJob{}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxBulkBody))
	if err != nil {
		return out, errors.New("the request body could not be read (at most 64 KiB)")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request bulkRequest
	if err := decoder.Decode(&request); err != nil {
		return out, fmt.Errorf("the body must be {\"action\": ..., \"ids\": [...]}: %v", err)
	}
	if decoder.More() {
		return out, errors.New("the body must be one JSON object")
	}
	if _, known := resource.actions[request.Action]; !known {
		names := make([]string, 0, len(resource.actions))
		for name := range resource.actions {
			names = append(names, name)
		}
		sort.Strings(names)
		return out, fmt.Errorf("action must be one of: %s", strings.Join(names, ", "))
	}
	seen := make(map[uint]struct{}, len(request.IDs))
	for _, id := range request.IDs {
		if id == 0 || id > 1<<32-1 {
			return out, errors.New("ids must be positive integers")
		}
		if _, repeated := seen[uint(id)]; repeated {
			continue
		}
		seen[uint(id)] = struct{}{}
		out.IDs = append(out.IDs, uint(id))
		if len(out.IDs) > maxBulkIDs {
			return out, fmt.Errorf("at most %d ids", maxBulkIDs)
		}
	}
	if len(out.IDs) == 0 {
		return out, errors.New("ids is required")
	}
	out.Action = request.Action
	return out, nil
}

// actorUserID is the administrator who sent the request.
func actorUserID(c *gin.Context) uint {
	switch id := c.Value("user_id").(type) {
	case uint:
		return id
	case int:
		if id > 0 {
			return uint(id)
		}
	}
	return 0
}

// safeBulkRequestID returns id when it can name a bridged request (letters,
// digits, ".", "_" and "-", at most 150 characters), else a fresh one.
func safeBulkRequestID(id string) string {
	if id != "" && len(id) <= 150 {
		safe := true
		for _, character := range id {
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
				continue
			}
			safe = false
			break
		}
		if safe {
			return id
		}
	}
	return uuid.NewString()
}

// bulkCapture keeps what the gateway writes: its body. The status is read
// from the gin context's writer.
type bulkCapture struct {
	header http.Header
	body   bytes.Buffer
}

func (w *bulkCapture) Header() http.Header            { return w.header }
func (w *bulkCapture) WriteHeader(int)                {}
func (w *bulkCapture) Write(data []byte) (int, error) { return w.body.Write(data) }

// GatewayBulkRunner runs each item through the /api/v2 package gateway
// (compatv2.Gateway.Serve, the handler of the single-item routes) with a
// request of its own built from the bulk request: the administrator's
// identity and client address, the item's method, path and id, and its own
// Idempotency-Key and request id. The router's middleware (rate limit,
// authentication, audit) is not run again: the bulk request passed it once,
// and the bulk handler audits each item itself.
func GatewayBulkRunner(engine *gin.Engine, gateway gin.HandlerFunc) BulkRunner {
	return func(ctx context.Context, parent *gin.Context, call BulkCall) BulkAnswer {
		capture := &bulkCapture{header: http.Header{}}
		sub := gin.CreateTestContextOnly(capture, engine)
		request := parent.Request.Clone(ctx)
		request.Method = call.Method
		request.URL = &url.URL{Path: call.Path}
		request.RequestURI = call.Path
		request.Body = http.NoBody
		request.ContentLength = 0
		request.Header = parent.Request.Header.Clone()
		request.Header.Del("Content-Type")
		request.Header.Del("Content-Length")
		request.Header.Set("Idempotency-Key", call.IdempotencyKey)
		request.Header.Set("X-Request-ID", call.RequestID)
		sub.Request = request
		sub.Params = call.Params
		for key, value := range parent.Keys {
			sub.Set(key, value)
		}
		sub.Set("request_id", call.RequestID)
		gateway(sub)
		return BulkAnswer{Status: sub.Writer.Status(), Body: capture.body.Bytes()}
	}
}

// BulkActionRoute names the single-item route a bulk action repeats.
type BulkActionRoute struct {
	// Endpoint is the bulk endpoint ("/api/v4/admin/users/bulk").
	Endpoint string
	Action   string
	Method   string
	// Pattern is the route as the router registers it (".../users/:id/ban").
	Pattern string
}

// BulkActionRoutes lists every bulk action with the single-item route it
// repeats, so the router's tests can check that each exists.
func BulkActionRoutes() []BulkActionRoute {
	var routes []BulkActionRoute
	for endpoint, resource := range map[string]bulkResource{
		"/api/v4/admin/users/bulk":        userBulkResource,
		"/api/v4/admin/invite-codes/bulk": inviteCodeBulkResource,
	} {
		for name, action := range resource.actions {
			routes = append(routes, BulkActionRoute{
				Endpoint: endpoint, Action: name, Method: action.method,
				Pattern: strings.Replace(action.path, "{id}", ":id", 1),
			})
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Endpoint != routes[j].Endpoint {
			return routes[i].Endpoint < routes[j].Endpoint
		}
		return routes[i].Action < routes[j].Action
	})
	return routes
}
