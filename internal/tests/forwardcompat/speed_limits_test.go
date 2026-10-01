package forwardcompat

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/forward/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// speedLimitRoute is a route of the kernel's SpeedLimitHandler, built per
// request like the other legacy handlers.
func speedLimitRoute(t *testing.T, pattern, routeID string, legacy func(*handler.SpeedLimitHandler, *gin.Context)) packagecompat.Route {
	return route(t, "POST", pattern, routeID, func(c *gin.Context) { legacy(handler.NewSpeedLimitHandler(), c) })
}

// withIdleLimits adds two speed limits no permission names, on epsilon and
// on the disabled gamma, to the given settings' seed.
func withIdleLimits(settings map[string]string) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seedWith(settings)(t, db)
		require.NoError(t, db.Create(&[]model.SpeedLimit{
			{ID: 3, CreatedTime: 3, UpdatedTime: 4, Status: 1, Name: "idle", Speed: 64, TunnelID: 5, TunnelName: "epsilon"},
			{ID: 4, CreatedTime: 5, UpdatedTime: 6, Status: 0, Name: "off", Speed: 32, TunnelID: 3, TunnelName: "gamma"},
		}).Error)
		syncSequences(t, db)
	}
}

// clockMillis is a speed limit time the handlers take from their clock (now,
// in milliseconds): masked unless it was seeded.
func clockMillis(value int64) any {
	if d := time.Since(time.UnixMilli(value)); d > -10*time.Minute && d < 10*time.Minute {
		return "<now>"
	}
	return value
}

// speedLimitState is state plus the speed limits.
func speedLimitState(t testing.TB, db *gorm.DB) any {
	all, ok := state(t, db).(map[string]any)
	require.True(t, ok)
	var limits []model.SpeedLimit
	require.NoError(t, db.Order("id").Find(&limits).Error)
	rows := make([]map[string]any, 0, len(limits))
	for _, limit := range limits {
		rows = append(rows, map[string]any{
			"id": limit.ID, "name": limit.Name, "speed": limit.Speed, "status": limit.Status, "tunnel_id": limit.TunnelID,
			"tunnel_name": limit.TunnelName, "created_time": clockMillis(limit.CreatedTime), "updated_time": clockMillis(limit.UpdatedTime),
		})
	}
	all["speed_limits"] = rows
	return all
}

func writeSpeedLimits(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	t.Helper()
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = withIdleLimits(nil)
		}
		c.Snapshot = speedLimitState
		packagecompat.RunWrite(t, r, c)
	}
}

func TestSpeedLimitCreateParity(t *testing.T) {
	r := speedLimitRoute(t, "/api/v2/speed-limit/create", native.SpeedLimitCreateRouteID, (*handler.SpeedLimitHandler).CreatePanelSpeedLimit)
	body := func(json string) []byte { return []byte(json) }
	writeSpeedLimits(t, r, []packagecompat.Case{
		{Name: "a limit", Path: r.Pattern, Principal: admin, Body: body(`{"name":" turbo ","speed":9000,"tunnelId":1,"tunnelName":" alpha "}`)},
		{Name: "a limit on a disabled tunnel", Path: r.Pattern, Principal: admin, Body: body(`{"name":"g","speed":1,"tunnelId":3,"tunnelName":"gamma"}`)},
		{Name: "a second limit on a tunnel", Path: r.Pattern, Principal: admin, Body: body(`{"name":"fast","speed":5000,"tunnelId":1,"tunnelName":"alpha"}`)},
		{Name: "unknown fields are ignored", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":2,"tunnelId":2,"tunnelName":"beta","status":0,"id":9}`)},
		{Name: "a tunnel name that does not match", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":2,"tunnelId":1,"tunnelName":"beta"}`)},
		{Name: "a tunnel name in another case", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":2,"tunnelId":7,"tunnelName":"alpha"}`)},
		{Name: "an unknown tunnel", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":2,"tunnelId":99,"tunnelName":"alpha"}`)},
		{Name: "no tunnel id", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":2,"tunnelName":"alpha"}`)},
		{Name: "a blank tunnel name", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":2,"tunnelId":1,"tunnelName":"  "}`)},
		{Name: "a blank name", Path: r.Pattern, Principal: admin, Body: body(`{"name":" ","speed":2,"tunnelId":1,"tunnelName":"alpha"}`)},
		{Name: "a zero speed", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":0,"tunnelId":1,"tunnelName":"alpha"}`)},
		{Name: "a negative speed", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":-1,"tunnelId":1,"tunnelName":"alpha"}`)},
		{Name: "the tunnel is checked first", Path: r.Pattern, Principal: admin, Body: body(`{"name":"","speed":0,"tunnelId":0,"tunnelName":""}`)},
		{Name: "a speed that is a string", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":"2","tunnelId":1,"tunnelName":"alpha"}`)},
		{Name: "a negative tunnel id", Path: r.Pattern, Principal: admin, Body: body(`{"name":"x","speed":2,"tunnelId":-1,"tunnelName":"alpha"}`)},
		{Name: "a body that is not an object", Path: r.Pattern, Principal: admin, Body: body(`[1]`)},
		{Name: "a body that does not parse", Path: r.Pattern, Principal: admin, Body: body(`{"name":`)},
		{Name: "no body", Path: r.Pattern, Principal: admin},
		{Name: "no tunnels", Path: r.Pattern, Principal: admin, Seed: empty, Body: body(`{"name":"x","speed":2,"tunnelId":1,"tunnelName":"alpha"}`)},
	})
}

func TestSpeedLimitListParity(t *testing.T) {
	r := speedLimitRoute(t, "/api/v2/speed-limit/list", native.SpeedLimitListRouteID, (*handler.SpeedLimitHandler).ListPanelSpeedLimits)
	read(t, r, []packagecompat.Case{
		{Name: "every limit by id", Path: r.Pattern, Principal: admin, Seed: withIdleLimits(nil)},
		{Name: "a body is ignored", Path: r.Pattern, Principal: admin, Seed: withIdleLimits(nil), Body: []byte(`{"tunnelId":1}`)},
		{Name: "no limits", Path: r.Pattern, Principal: admin, Seed: empty},
	})
}

func TestSpeedLimitDeleteParity(t *testing.T) {
	r := speedLimitRoute(t, "/api/v2/speed-limit/delete", native.SpeedLimitDeleteRouteID, (*handler.SpeedLimitHandler).DeletePanelSpeedLimit)
	writeSpeedLimits(t, r, []packagecompat.Case{
		{Name: "an unused limit", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":3}`)},
		{Name: "an unused inactive limit", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":4}`)},
		{Name: "a limit a permission names", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":1}`)},
		{Name: "a limit another user's permission names", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":2}`)},
		{Name: "an unknown limit", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":99}`)},
		{Name: "deleting twice", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":3}`), Warmup: [][]byte{[]byte(`{"id":3}`)}},
		{Name: "limit zero", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":0}`)},
		{Name: "no id", Path: r.Pattern, Principal: admin, Body: []byte(`{}`)},
		{Name: "an id that is a string", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":"3"}`)},
		{Name: "a negative id", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":-3}`)},
		{Name: "an id beyond 32 bits", Path: r.Pattern, Principal: admin, Body: []byte(`{"id":4294967299}`)},
		{Name: "no body", Path: r.Pattern, Principal: admin},
	})
}

func TestSpeedLimitTunnelsParity(t *testing.T) {
	r := speedLimitRoute(t, "/api/v2/speed-limit/tunnels", native.SpeedLimitTunnelsRouteID, (*handler.SpeedLimitHandler).ListPanelSpeedLimitTunnels)
	read(t, r, []packagecompat.Case{
		{Name: "every active tunnel", Path: r.Pattern, Principal: admin},
		{Name: "every active tunnel whoever asks", Path: r.Pattern, Principal: alice},
		{Name: "no caller", Path: r.Pattern, Principal: pluginhostsdk.Principal{}},
		{Name: "NodeX mode", Path: r.Pattern, Principal: admin, Seed: seedWith(nodeXOn)},
		{Name: "the Ansible backend", Path: r.Pattern, Principal: admin, Seed: seedWith(ansibleOwn)},
		{Name: "clean agents", Path: r.Pattern, Principal: admin, Seed: seedWith(cleanAgent)},
		{Name: "iptables", Path: r.Pattern, Principal: admin, Seed: seedWith(iptables)},
		{Name: "an unknown backend", Path: r.Pattern, Principal: admin, Seed: seedWith(badBackend)},
		{Name: "no tunnels", Path: r.Pattern, Principal: admin, Seed: empty},
	})
}
