package native

import (
	"context"
	"fmt"
	"net/http"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native/model"
	"github.com/gin-gonic/gin/binding"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Registration keys mint node credentials, so they are the kernel's
// (v2_authorized_key stays protected): the package lists them through
// kapi_registration_key_v1, which the kernel grants once the table is
// finalized and which shows whether a key holds a value, never the value;
// it issues and revokes them through KernelNodeOps (IssueRegistrationKey,
// RevokeRegistrationKey). A new key is answered as a sealed handle the
// kernel's gateway expands in the answer, shown once.
const registrationKeyView = "kapi_registration_key_v1"

// RegistrationKey is a row of kapi_registration_key_v1.
type RegistrationKey struct {
	ID        uint
	Name      string
	Used      int
	ExpireAt  *int64
	CreatedAt time.Time
	UpdatedAt time.Time
	HasKey    bool
}

// TableName is the kernel view.
func (RegistrationKey) TableName() string { return registrationKeyView }

// ListAuthKeys is GET /api/v2/admin/auth-keys: the registration keys, the
// newest first, each key as the placeholder.
func (s *Service) ListAuthKeys(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	if !s.leased(ctx, registrationKeyView) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var rows []RegistrationKey
	if err := db.Order("created_at DESC").Find(&rows).Error; err != nil {
		return message(http.StatusInternalServerError, "获取失败")
	}
	keys := make([]model.AuthorizedKey, 0, len(rows))
	for _, row := range rows {
		key := model.AuthorizedKey{ID: row.ID, Name: row.Name, Used: row.Used, ExpireAt: row.ExpireAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
		if row.HasKey {
			key.Key = v2compat.NodeSecretPlaceholder
		}
		keys = append(keys, key)
	}
	return s.panel(keys)
}

// registrationKeyExpiry is the kernel's: the end of a key that lasts
// expireDays, none for zero.
func registrationKeyExpiry(expireDays int, now time.Time) int64 {
	if expireDays <= 0 {
		return 0
	}
	return now.Add(time.Duration(expireDays) * 24 * time.Hour).Unix()
}

// issueRegistrationKey issues a key through the kernel and answers it as
// the routes show it: id, name, the key's handle and its end.
func (s *Service) issueRegistrationKey(ctx context.Context, request pluginhostsdk.NativeRequest, name string, expireDays int) (map[string]any, bool, error) {
	operation, err := s.submit(ctx, request, s.requestID(request, "regkey.issue", true),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueRegistrationKey{IssueRegistrationKey: &kernelnodeopsv1.IssueRegistrationKey{
			Name: name, ExpiresAtUnix: registrationKeyExpiry(expireDays, s.now()),
		}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, kernelOperationWait)
	if err != nil {
		return nil, false, err
	}
	result := operation.GetResult().GetRegistrationKey()
	if !succeeded(operation) || result == nil {
		return nil, false, nil
	}
	var handle string
	for _, reveal := range result.GetReveal() {
		if reveal.GetField() == "key" {
			handle = reveal.GetHandle()
		}
	}
	var expireAt *int64
	if result.GetExpiresAtUnix() != 0 {
		value := result.GetExpiresAtUnix()
		expireAt = &value
	}
	return map[string]any{"id": uint(result.GetKeyId()), "name": result.GetName(), "key": handle, "expire_at": expireAt}, true, nil // #nosec G115 -- key ids are 32-bit.
}

// GenerateAuthKey is POST /api/v2/admin/auth-keys.
func (s *Service) GenerateAuthKey(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Name       string `json:"name" binding:"required,min=1,max=255"`
		ExpireDays int    `json:"expire_days" binding:"gte=0"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return message(http.StatusBadRequest, "参数错误")
	}
	if s.NodeOps == nil || len(request.Binding) == 0 {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if req.Name == "" {
		req.Name = "授权密钥"
	}
	answer, ok, err := s.issueRegistrationKey(ctx, request, req.Name, req.ExpireDays)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if !ok {
		return message(http.StatusInternalServerError, "生成失败")
	}
	return s.panel(answer)
}

// InternalGenerateAuthKey is POST /api/v2/internal/auth-keys, for
// automation authenticated by an API token: its answer is not the panel
// envelope.
func (s *Service) InternalGenerateAuthKey(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Name       string `json:"name" binding:"min=1,max=255"`
		ExpireDays int    `json:"expire_days" binding:"gte=0"`
		NodeName   string `json:"node_name" binding:"omitempty,max=255"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return message(http.StatusBadRequest, "参数错误")
	}
	if s.NodeOps == nil || len(request.Binding) == 0 {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if req.Name == "" {
		if req.NodeName != "" {
			req.Name = "Ansible-" + req.NodeName
		} else {
			req.Name = "Ansible-Auto-Generated"
		}
	}
	answer, ok, err := s.issueRegistrationKey(ctx, request, req.Name, req.ExpireDays)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if !ok {
		return message(http.StatusInternalServerError, "生成失败")
	}
	return jsonAnswer(http.StatusOK, map[string]any{"message": "生成成功", "data": answer})
}

// DeleteAuthKey is DELETE /api/v2/admin/auth-keys/:id: the kernel revokes
// the key (RevokeRegistrationKey); a key that does not exist is nothing to
// do.
func (s *Service) DeleteAuthKey(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return message(http.StatusBadRequest, "无效的ID")
	}
	if s.NodeOps == nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	operation, err := s.submit(ctx, request, fmt.Sprintf("regkey.revoke:%d", id),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RevokeRegistrationKey{RevokeRegistrationKey: &kernelnodeopsv1.RevokeRegistrationKey{KeyId: uint64(id)}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, kernelOperationWait)
	switch {
	case status.Code(err) == codes.NotFound:
	case err != nil:
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	case !succeeded(operation):
		return message(http.StatusInternalServerError, "删除失败")
	}
	return s.panel(map[string]any{"message": "删除成功"})
}
