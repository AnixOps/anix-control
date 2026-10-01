package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Gateway types, as the kernel's model.PaymentGateway* constants.
const (
	gatewayEPay   = "epay"
	gatewayStripe = "stripe"
	gatewayPayPal = "paypal"
	gatewayX402   = "x402"
)

// enabledGatewayTypes are the gateway types with a live callback, the only
// ones that may be enabled.
var enabledGatewayTypes = []string{gatewayEPay, gatewayStripe, gatewayPayPal, gatewayX402}

func gatewayTypeCanBeEnabled(gatewayType string) bool {
	for _, enabled := range enabledGatewayTypes {
		if gatewayType == enabled {
			return true
		}
	}
	return false
}

// validateGatewayCanBeEnabled is the kernel's
// validatePaymentGatewayCanBeEnabled.
func validateGatewayCanBeEnabled(gateway *PaymentGateway) error {
	if gateway == nil || !gateway.Enabled {
		return nil
	}
	if gatewayTypeCanBeEnabled(gateway.Type) {
		return nil
	}
	return fmt.Errorf("payment gateway type %q cannot be enabled until live callback implementation and tests are complete", gateway.Type)
}

// CreateGatewayRequest is the kernel's request type of the same name; the
// name appears in binding errors.
type CreateGatewayRequest struct {
	Name        string  `json:"name" binding:"required"`
	Type        string  `json:"type" binding:"required,oneof=alipay wechat stripe usdt epay"`
	Icon        string  `json:"icon"`
	Config      any     `json:"config"`
	FeeRate     float64 `json:"fee_rate"`
	FeeFixed    float64 `json:"fee_fixed"`
	MinAmount   float64 `json:"min_amount"`
	MaxAmount   float64 `json:"max_amount"`
	Sort        int     `json:"sort"`
	Description string  `json:"description"`
}

// UpdateGatewayRequest is the kernel's request type of the same name.
type UpdateGatewayRequest struct {
	Name        string   `json:"name"`
	Type        string   `json:"type" binding:"omitempty,oneof=alipay wechat stripe usdt epay"`
	Icon        string   `json:"icon"`
	Config      any      `json:"config"`
	FeeRate     *float64 `json:"fee_rate"`
	FeeFixed    *float64 `json:"fee_fixed"`
	MinAmount   *float64 `json:"min_amount"`
	MaxAmount   *float64 `json:"max_amount"`
	Sort        *int     `json:"sort"`
	Description string   `json:"description"`
}

// normalizeGatewayConfig stores a configuration sent as a string as is, and
// any other JSON value encoded.
func normalizeGatewayConfig(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	if str, ok := raw.(string); ok {
		return str, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// gatewayID parses the :id path parameter as the legacy handlers do.
func gatewayID(request pluginhostsdk.NativeRequest) (uint, bool) {
	id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
	return uint(id), err == nil
}

// AdminGateways is GET /api/v2/admin/payment/gateways, secrets redacted.
func (s *Service) AdminGateways(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var gateways []*PaymentGateway
	if err := db.Order("sort ASC, id ASC").Find(&gateways).Error; err != nil {
		return s.panelError(err.Error())
	}
	for _, gateway := range gateways {
		gateway.Config = redactGatewayConfig(gateway.Config)
	}
	return s.panel(map[string]any{"list": gateways, "total": len(gateways)})
}

// AdminCreateGateway is POST /api/v2/admin/payment/gateways. A new gateway
// is disabled; a secret sent as the placeholder is stored empty.
func (s *Service) AdminCreateGateway(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req CreateGatewayRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	gateway := &PaymentGateway{
		Name: req.Name, Type: req.Type, Enabled: false, Icon: req.Icon, FeeRate: req.FeeRate, FeeFixed: req.FeeFixed,
		MinAmount: req.MinAmount, MaxAmount: req.MaxAmount, Sort: req.Sort, Description: req.Description,
	}
	config, err := normalizeGatewayConfig(req.Config)
	if err != nil {
		return s.panelError("invalid config")
	}
	gateway.Config = keepGatewaySecrets(config, "")
	if err := validateGatewayCanBeEnabled(gateway); err != nil {
		return s.panelError(err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := db.Create(gateway).Error; err != nil {
		return s.panelError(err.Error())
	}
	gateway.Config = redactGatewayConfig(gateway.Config)
	return s.panel(gateway)
}

// AdminUpdateGateway is PUT /api/v2/admin/payment/gateways/:id: the fields
// sent replace the stored ones, a secret sent as the placeholder keeps its
// stored value, and the whole row is saved.
func (s *Service) AdminUpdateGateway(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := gatewayID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("gateway not found")
	}
	var gateway PaymentGateway
	if err := db.First(&gateway, id).Error; err != nil {
		return s.panelError("gateway not found")
	}
	var req UpdateGatewayRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	if req.Name != "" {
		gateway.Name = req.Name
	}
	if req.Type != "" {
		gateway.Type = req.Type
	}
	if req.Icon != "" {
		gateway.Icon = req.Icon
	}
	if req.Config != nil {
		config, err := normalizeGatewayConfig(req.Config)
		if err != nil {
			return s.panelError("invalid config")
		}
		gateway.Config = keepGatewaySecrets(config, gateway.Config)
	}
	if req.FeeRate != nil {
		gateway.FeeRate = *req.FeeRate
	}
	if req.FeeFixed != nil {
		gateway.FeeFixed = *req.FeeFixed
	}
	if req.MinAmount != nil {
		gateway.MinAmount = *req.MinAmount
	}
	if req.MaxAmount != nil {
		gateway.MaxAmount = *req.MaxAmount
	}
	if req.Sort != nil {
		gateway.Sort = *req.Sort
	}
	if req.Description != "" {
		gateway.Description = req.Description
	}
	if err := validateGatewayCanBeEnabled(&gateway); err != nil {
		return s.panelError(err.Error())
	}
	if err := db.Save(&gateway).Error; err != nil {
		return s.panelError(err.Error())
	}
	gateway.Config = redactGatewayConfig(gateway.Config)
	return s.panel(&gateway)
}

// AdminDeleteGateway is DELETE /api/v2/admin/payment/gateways/:id; deleting
// a gateway that does not exist succeeds.
func (s *Service) AdminDeleteGateway(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := gatewayID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := db.Delete(&PaymentGateway{}, id).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"message": "deleted"})
}

// AdminToggleGateway is POST /api/v2/admin/payment/gateways/:id/toggle: the
// state sent as "enabled", else the opposite of the current one. Only a
// gateway type with a live callback may be enabled.
func (s *Service) AdminToggleGateway(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := gatewayID(request)
	if !ok {
		return s.panelError("invalid id")
	}
	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil && !errors.Is(err, io.EOF) {
		return s.panelError(err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	targetEnabled := false
	if rawEnabled, ok := req["enabled"]; ok {
		enabled, ok := rawEnabled.(bool)
		if !ok {
			return s.panelError("enabled must be boolean")
		}
		targetEnabled = enabled
	} else {
		var gateway PaymentGateway
		if err := db.First(&gateway, id).Error; err != nil {
			return s.panelError("gateway not found")
		}
		targetEnabled = !gateway.Enabled
	}
	if targetEnabled {
		var gateway PaymentGateway
		if err := db.First(&gateway, id).Error; err != nil {
			return s.panelError(err.Error())
		}
		gateway.Enabled = true
		if err := validateGatewayCanBeEnabled(&gateway); err != nil {
			return s.panelError(err.Error())
		}
	}
	if err := db.Model(&PaymentGateway{}).Where("id = ?", id).Update("enabled", targetEnabled).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"enabled": targetEnabled})
}

// enabledGateways are the gateways users may pay with.
func enabledGateways(db *gorm.DB) ([]*PaymentGateway, error) {
	var gateways []*PaymentGateway
	err := db.Where("enabled = ? AND type IN ?", true, enabledGatewayTypes).Order("sort ASC, id ASC").Find(&gateways).Error
	return gateways, err
}

// UserChannels is GET /api/v2/user/payment/channels: the enabled gateways,
// without their configuration.
func (s *Service) UserChannels(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	gateways, err := enabledGateways(db)
	if err != nil {
		return s.panelError(err.Error())
	}
	channels := make([]*PaymentChannel, len(gateways))
	for i, g := range gateways {
		channels[i] = &PaymentChannel{
			ID: g.ID, Name: g.Name, Type: g.Type, Icon: g.Icon, MinAmount: g.MinAmount, MaxAmount: g.MaxAmount,
			FeeRate: g.FeeRate, FeeFixed: g.FeeFixed, Description: g.Description,
		}
	}
	return s.panel(channels)
}
