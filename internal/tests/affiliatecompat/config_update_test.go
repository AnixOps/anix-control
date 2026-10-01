package affiliatecompat

import (
	"context"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/affiliate/native"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// configUpdate is the invite configuration update: the native side writes
// the adopted table and the frontend settings through the real
// KernelSettings server on its database, as the affiliate host, whose
// signed release declares kernel.settings.invite.write.v1.
func configUpdate(t *testing.T) packagecompat.Route {
	r := route(t, "PUT", "/api/v2/admin/invite/config", native.ConfigUpdateRouteID, (*handler.InviteHandler).UpdateConfig)
	r.Models = append(r.Models, &model.OperationLog{}, &model.SettingsRequest{})
	r.Native = func(db *gorm.DB) pluginhostsdk.NativeHandler {
		service := &native.Service{
			Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
			KernelSettings: packagecompat.KernelSettings(t, db, packagecompat.SettingsGrants{
				Host: affiliateHost, Capabilities: []string{service.SettingsCapability(service.SettingsNamespaceInvite, service.SettingsAccessWrite)},
			}),
		}
		return service.Handlers()[native.ConfigUpdateRouteID]
	}
	return r
}

// inviteCopies are the kernel's invite services by database: each loaded
// the configuration into memory before the request, as the kernel's
// long-lived ones have (the user routes' handlers, the identity bridge's).
var inviteCopies sync.Map

func warmInvite(seed func(testing.TB, *gorm.DB)) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seed(t, db)
		invites := service.NewInviteService(db)
		_, err := invites.GetConfig()
		require.NoError(t, err)
		inviteCopies.Store(db, invites)
	}
}

// configState is the affiliate state, v2_system_config, the number of
// audit entries (neither side records one) and the invite configuration
// the kernel's invite service holds in memory after the request.
func configState(t testing.TB, db *gorm.DB) any {
	var configs []struct {
		ID     uint
		Key    string
		Value  string
		Type   string
		Group  string
		Remark string
	}
	require.NoError(t, db.Model(&model.SystemConfig{}).Order("id").Find(&configs).Error)
	var audit int64
	require.NoError(t, db.Model(&model.OperationLog{}).Count(&audit).Error)
	invites := service.NewInviteService(db)
	if warmed, ok := inviteCopies.Load(db); ok {
		invites = warmed.(*service.InviteService)
	}
	memory, err := invites.GetConfig()
	require.NoError(t, err)
	copied := *memory
	copied.CreatedAt, copied.UpdatedAt = seeded, seeded
	return map[string]any{"affiliate": state(t, db), "settings": configs, "audit": audit, "memory": copied}
}

func TestAdminConfigUpdateParity(t *testing.T) {
	stored := warmInvite(seed)
	clock := []string{"data.updated_at"}
	for _, c := range []packagecompat.Case{
		{Name: "every field, loosely typed", Seed: stored, Mask: clock, Body: []byte(`{"enabled":"no","commission_enabled":0,"auto_generate":"on",
			"code_count":"7","code_expire_days":14.9,"commission_type":"percentage","commission_rate":25,"commission_fixed":"1.5",
			"commission_min":3,"first_order_bonus":0.5,"first_traffic_bonus":"1024","code_prefix":"  VIP ","code_length":"10",
			"withdraw_fee":"0.75","withdraw_methods":"bank, alipay ,"}`)},
		{Name: "commission_rate_ratio wins over commission_rate", Seed: stored, Mask: clock, Body: []byte(`{"commission_rate_ratio":0.35,"commission_rate":80}`)},
		{Name: "min_withdraw wins over commission_min", Seed: stored, Mask: clock, Body: []byte(`{"commission_min":5,"min_withdraw":8}`)},
		{Name: "zero and blank commission types are unspecified", Seed: stored, Mask: clock, Body: []byte(`{"commission_type":" ","commission_type_code":0}`)},
		{Name: "commission_type_code wins", Seed: stored, Mask: clock, Body: []byte(`{"commission_type":"fixed","commission_type_code":1}`)},
		{Name: "an empty prefix answers empty and stores the default", Seed: stored, Mask: clock, Body: []byte(`{"code_prefix":"","withdraw_methods":[" bank ",""]}`)},
		{Name: "withdraw methods as a JSON array in a string", Seed: stored, Mask: clock, Body: []byte(`{"withdraw_methods":"[\"usdt\",\"bank\"]"}`)},
		{Name: "an empty update saves as it is", Seed: stored, Mask: clock, Body: []byte(`{}`)},
		{Name: "a null body", Seed: stored, Mask: clock, Body: []byte(`null`)},
		{Name: "no configuration and no settings yet", Seed: seedWith(false, nil), Mask: []string{"data.created_at", "data.updated_at"}, Body: []byte(`{"code_length":6}`)},
		{Name: "settings that are not JSON read as the defaults", Seed: warmInvite(seedWith(true, ptr(`{"code_prefix":`))), Mask: clock, Body: []byte(`{"withdraw_fee":2}`)},
		{Name: "not a boolean", Seed: stored, Body: []byte(`{"enabled":"maybe"}`)},
		{Name: "a negative code count", Seed: stored, Body: []byte(`{"code_count":-1}`)},
		{Name: "an expiry that is not a number", Seed: stored, Body: []byte(`{"code_expire_days":"soon"}`)},
		{Name: "an unknown commission type", Seed: stored, Body: []byte(`{"commission_type":"weekly"}`)},
		{Name: "a commission type out of range", Seed: stored, Body: []byte(`{"commission_type":3}`)},
		{Name: "a commission type of another type", Seed: stored, Body: []byte(`{"commission_type":true}`)},
		{Name: "a commission type code out of range", Seed: stored, Body: []byte(`{"commission_type_code":"9"}`)},
		{Name: "a negative ratio", Seed: stored, Body: []byte(`{"commission_rate_ratio":-0.1}`)},
		{Name: "a negative rate", Seed: stored, Body: []byte(`{"commission_rate":"-1"}`)},
		{Name: "a negative fixed commission", Seed: stored, Body: []byte(`{"commission_fixed":-1}`)},
		{Name: "a minimum that is not a number", Seed: stored, Body: []byte(`{"min_withdraw":"lots"}`)},
		{Name: "a negative first order bonus", Seed: stored, Body: []byte(`{"first_order_bonus":-2}`)},
		{Name: "a negative traffic bonus", Seed: stored, Body: []byte(`{"first_traffic_bonus":-2}`)},
		{Name: "a prefix that is not a string", Seed: stored, Body: []byte(`{"code_prefix":5}`)},
		{Name: "a code length of zero", Seed: stored, Body: []byte(`{"code_length":0}`)},
		{Name: "a negative fee", Seed: stored, Body: []byte(`{"withdraw_fee":-0.5}`)},
		{Name: "no withdraw methods", Seed: stored, Body: []byte(`{"withdraw_methods":[]}`)},
		{Name: "withdraw methods that are not strings", Seed: stored, Body: []byte(`{"withdraw_methods":[1]}`)},
		{Name: "the first refused field answers", Seed: stored, Body: []byte(`{"withdraw_fee":-1,"code_count":-1}`)},
		{Name: "a refused field still creates the defaults", Seed: seedWith(false, nil), Body: []byte(`{"code_length":-1}`)},
		{Name: "a body that is not an object", Seed: stored, Body: []byte(`[1]`)},
		{Name: "a body that does not parse", Seed: stored, Body: []byte(`{"enabled":`)},
	} {
		c.Path = "/api/v2/admin/invite/config"
		c.Principal = admin
		c.Snapshot = configState
		packagecompat.RunWrite(t, configUpdate(t), c)
	}
}
