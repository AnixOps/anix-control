package native

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/AnixOps/anix-control/identity/account"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AdminCreateUserRequest binds exactly as the v2 request of the same name,
// whose type name appears in validation messages.
type AdminCreateUserRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required,min=6"`
	IsAdmin        int    `json:"is_admin"`
	FlowResetTime  int64  `json:"flowResetTime"`
	GroupID        *uint  `json:"group_id"`
	TransferEnable *int64 `json:"transfer_enable"`
	SpeedLimit     *int64 `json:"speed_limit"`
	DeviceLimit    *int   `json:"device_limit"`
}

// CreateUser is POST /api/v2/admin/users: the account in identity, the
// subscriber and its entitlements in Control, answered with Control's user.
func (s *Service) CreateUser(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req AdminCreateUserRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if taken, err := stores.Accounts.EmailTaken(ctx, req.Email); err == nil && taken {
		return s.panelError("该邮箱已被注册")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return s.panelError("密码加密失败")
	}
	failed := func(err error) (pluginhostsdk.NativeResponse, error) {
		return s.panelError("创建用户失败: " + status.Convert(err).Message())
	}
	accountUUID := uuid.NewString()
	created, err := s.Kernel.CreateSubscriber(ctx, &kernelidentityv1.CreateSubscriberRequest{
		AccountUuid: accountUUID, Email: req.Email, IsAdmin: req.IsAdmin != 0,
		LegacyMirror: &kernelidentityv1.LegacyCredentialMirror{PasswordHash: string(hash)},
	})
	if err != nil {
		return failed(err)
	}
	entitlements := map[string]any{"flowResetTime": req.FlowResetTime}
	if req.GroupID != nil {
		entitlements["group_id"] = *req.GroupID
	}
	if req.TransferEnable != nil {
		entitlements["transfer_enable"] = *req.TransferEnable
	}
	if req.SpeedLimit != nil {
		entitlements["speed_limit"] = *req.SpeedLimit
	}
	if req.DeviceLimit != nil {
		entitlements["device_limit"] = *req.DeviceLimit
	}
	if err := s.updateSubscriber(ctx, created.GetUserId(), entitlements); err != nil {
		return failed(err)
	}
	user := account.Account{
		UserID: created.GetUserId(), AccountUUID: accountUUID, Email: req.Email, PasswordHash: string(hash), IsAdmin: req.IsAdmin != 0,
	}
	// The subscriber was created with the account's flags: nothing to project.
	if err := stores.Accounts.Create(ctx, user); err != nil {
		return failed(err)
	}
	return s.subscriber(ctx, created.GetUserId())
}

func (s *Service) subscriber(ctx context.Context, userID uint64) (pluginhostsdk.NativeResponse, error) {
	shown, err := s.Kernel.GetSubscriber(ctx, &kernelidentityv1.GetSubscriberRequest{UserId: userID})
	if err != nil {
		return s.panelError(status.Convert(err).Message())
	}
	return s.panel(json.RawMessage(shown.GetSubscriberJson()))
}

func (s *Service) updateSubscriber(ctx context.Context, userID uint64, entitlements map[string]any) error {
	encoded, err := json.Marshal(entitlements)
	if err != nil {
		return err
	}
	_, err = s.Kernel.UpdateSubscriber(ctx, &kernelidentityv1.UpdateSubscriberRequest{UserId: userID, EntitlementsJson: encoded})
	return err
}

// mirrorMFA mirrors an account's second factor to Control's legacy tables
// after it changed, so switching back to the legacy routes keeps it.
func (s *Service) mirrorMFA(ctx context.Context, stores *Stores, userID uint64) error {
	touched, err := stores.Accounts.Touch(ctx, userID)
	if err != nil {
		return err
	}
	return s.project(ctx, stores, touched, true)
}

// project writes an account's identity fields into Control's projection.
// With mirror, the password hash and MFA state are mirrored to the legacy
// columns too, so switching back to the legacy routes keeps logins working.
func (s *Service) project(ctx context.Context, stores *Stores, a account.Account, mirror bool) error {
	request := &kernelidentityv1.ApplyAccountProjectionRequest{
		UserId: a.UserID, Version: a.Version, Email: a.Email, IsAdmin: a.IsAdmin, IsStaff: a.IsStaff, Banned: a.Banned,
	}
	if mirror {
		credentials := &kernelidentityv1.LegacyCredentialMirror{PasswordHash: a.PasswordHash, PasswordAlgo: a.PasswordAlgo, PasswordSalt: a.PasswordSalt}
		mfa, _, err := stores.Accounts.MFA(ctx, a.UserID)
		if err != nil {
			return err
		}
		if mfa != nil && mfa.Enabled {
			credentials.MfaEnabled, credentials.TotpSecret = true, mfa.TOTPSecret
		}
		request.LegacyMirror = credentials
	}
	_, err := s.Kernel.ApplyAccountProjection(ctx, request)
	return err
}

func parseUserID(raw string) (uint64, bool) {
	id, err := strconv.ParseUint(raw, 10, 32)
	return id, err == nil
}

// UpdateUser is PUT /api/v2/admin/users/:id: identity fields in identity
// (projected to Control), entitlements in Control.
func (s *Service) UpdateUser(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseUserID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的用户ID")
	}
	var req struct {
		Email          *string `json:"email"`
		Password       *string `json:"password"`
		Balance        *int64  `json:"balance"`
		PlanID         *uint   `json:"plan_id" binding:"omitempty,gt=0"`
		GroupID        *uint   `json:"group_id"`
		ExpiredAt      *int64  `json:"expired_at"`
		TransferEnable *int64  `json:"transfer_enable"`
		SpeedLimit     *int64  `json:"speed_limit"`
		DeviceLimit    *int    `json:"device_limit"`
		FlowResetTime  *int64  `json:"flowResetTime"`
		Banned         *int    `json:"banned"`
		IsAdmin        *int    `json:"is_admin"`
		RemarkContent  *string `json:"remark_content"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	changes := account.Changes{Email: req.Email}
	if req.Password != nil && *req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return s.panelError("密码加密失败")
		}
		hashed := string(hash)
		changes.PasswordHash = &hashed
	}
	if req.Banned != nil {
		banned := *req.Banned != 0
		changes.Banned = &banned
	}
	if req.IsAdmin != nil {
		admin := *req.IsAdmin != 0
		changes.IsAdmin = &admin
	}
	entitlements := map[string]any{}
	for name, value := range map[string]any{
		"balance": req.Balance, "plan_id": req.PlanID, "group_id": req.GroupID, "expired_at": req.ExpiredAt,
		"transfer_enable": req.TransferEnable, "speed_limit": req.SpeedLimit, "device_limit": req.DeviceLimit,
		"flowResetTime": req.FlowResetTime, "remark_content": req.RemarkContent,
	} {
		if !isNilPointer(value) {
			entitlements[name] = value
		}
	}
	if response, failed := s.changeAccount(ctx, id, changes, "更新失败"); failed {
		return response, nil
	}
	if len(entitlements) > 0 {
		if err := s.updateSubscriber(ctx, id, entitlements); err != nil {
			if status.Code(err) == codes.NotFound {
				return s.panelError("用户不存在")
			}
			return s.panelError("更新失败: " + status.Convert(err).Message())
		}
	}
	return s.panel(map[string]any{"message": "更新成功"})
}

func isNilPointer(value any) bool {
	switch typed := value.(type) {
	case *int64:
		return typed == nil
	case *uint:
		return typed == nil
	case *int:
		return typed == nil
	case *string:
		return typed == nil
	default:
		return value == nil
	}
}

// changeAccount updates identity fields and projects them; it answers the
// failure itself (failed true) with the v2 message.
func (s *Service) changeAccount(ctx context.Context, id uint64, changes account.Changes, failure string) (pluginhostsdk.NativeResponse, bool) {
	stores, err := s.Open(ctx)
	if err != nil {
		response, _ := s.panelError(err.Error())
		return response, true
	}
	result, err := stores.Accounts.Update(ctx, id, changes)
	if errors.Is(err, account.ErrAccountNotFound) {
		response, _ := s.panelError("用户不存在")
		return response, true
	}
	if err != nil {
		response, _ := s.panelError(failure + ": " + err.Error())
		return response, true
	}
	if result.Changed {
		if err := s.project(ctx, stores, result.Account, result.PasswordChanged); err != nil {
			response, _ := s.panelError(failure + ": " + status.Convert(err).Message())
			return response, true
		}
	}
	return pluginhostsdk.NativeResponse{}, false
}

func (s *Service) setBanned(ctx context.Context, request pluginhostsdk.NativeRequest, banned bool, failure, success string) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseUserID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的用户ID")
	}
	if response, failed := s.changeAccount(ctx, id, account.Changes{Banned: &banned}, failure); failed {
		return response, nil
	}
	return s.panel(map[string]any{"message": success})
}

// BanUser is POST /api/v2/admin/users/:id/ban.
func (s *Service) BanUser(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	return s.setBanned(ctx, request, true, "封禁失败", "封禁成功")
}

// UnbanUser is POST /api/v2/admin/users/:id/unban.
func (s *Service) UnbanUser(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	return s.setBanned(ctx, request, false, "解封失败", "解封成功")
}

// DeleteUser is DELETE /api/v2/admin/users/:id: Control deletes the
// subscriber (and revokes its tokens), then identity the account.
func (s *Service) DeleteUser(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseUserID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的用户ID")
	}
	if id == 0 {
		return s.panelError("用户不存在")
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if _, err := s.Kernel.DeleteSubscriber(ctx, &kernelidentityv1.DeleteSubscriberRequest{UserId: id}); err != nil {
		if status.Code(err) == codes.NotFound {
			return s.panelError("用户不存在")
		}
		return s.panelError("删除失败: " + status.Convert(err).Message())
	}
	if err := stores.Accounts.Delete(ctx, id); err != nil {
		return s.panelError("删除失败: " + err.Error())
	}
	return s.panel(map[string]any{"message": "删除成功"})
}
