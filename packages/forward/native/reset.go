package native

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// ResetRequestID names an administrator's subscriber traffic reset through
// POST /api/v2/user/reset in Control's subscriber request ledger, so a
// retried request is applied once. token identifies the HTTP request: its
// Idempotency-Key, else its request id. The kernel's legacy handler derives
// the same id (service.ForwardUserResetRequestID), so a retry is recognized
// whichever side serves it; a new request is a new reset, as in v2.
func ResetRequestID(userID uint64, token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("forward.reset_traffic:%d:%x", userID, sum[:12])
}

// requestToken is what identifies the request for ResetRequestID: its
// Idempotency-Key, else its request id, else a fresh value.
func (s *Service) requestToken(request pluginhostsdk.NativeRequest) string {
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		for key, values := range request.Metadata.Headers {
			if strings.EqualFold(key, name) && len(values) > 0 {
				if value := strings.TrimSpace(values[0]); value != "" {
					return value
				}
			}
		}
	}
	if s.NewToken != nil {
		return s.NewToken()
	}
	return uuid.NewString()
}

// ResetFlow is POST /api/v2/user/reset, the administrator's Flux-style
// reset: type 1 zeroes a subscriber's traffic through
// KernelSubscriber.ResetTraffic, type 2 a tunnel permission's traffic and
// that of the permission's forwards. Neither changes what a node runs.
func (s *Service) ResetFlow(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		ID   uint `json:"id" binding:"required"`
		Type int  `json:"type" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	if req.Type != 1 && req.Type != 2 {
		return s.panelError("type must be 1 or 2")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	switch req.Type {
	case 1:
		var user DirectoryUser
		if err := db.Select("id").First(&user, req.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return s.panelError("用户不存在")
			}
			return s.panelError(err.Error())
		}
		if _, err := s.Subscriber.ResetTraffic(ctx, &kernelsubscriberv1.ResetTrafficRequest{
			RequestId: ResetRequestID(uint64(req.ID), s.requestToken(request)), UserIds: []uint64{uint64(req.ID)},
			Reason: "administrator reset",
		}); err != nil {
			return s.panelError(status.Convert(err).Message())
		}
	default:
		if err := resetUserTunnelTraffic(db, req.ID); err != nil {
			return s.panelError(err.Error())
		}
	}
	return s.panel(nil)
}

// resetUserTunnelTraffic zeroes a tunnel permission's traffic and that of
// its user's forwards on its tunnel, in one transaction.
func resetUserTunnelTraffic(db *gorm.DB, id uint) error {
	var record UserTunnel
	if err := db.First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("user tunnel permission not found")
		}
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Forward{}).Where("user_id = ? AND tunnel_id = ?", record.UserID, record.TunnelID).
			Updates(map[string]any{"in_flow": 0, "out_flow": 0}).Error; err != nil {
			return err
		}
		return tx.Model(&UserTunnel{}).Where("id = ?", record.ID).Updates(map[string]any{"in_flow": 0, "out_flow": 0}).Error
	})
}
