// Package native implements two of the machine-telemetry package's routes in
// the package itself: the administrator's hourly traffic series and user
// traffic ranking. They read the kernel's traffic log through the read-only
// kernel view kapi_traffic_log_v1 (the user, bytes, rate and time of each
// node traffic report in v2_server_log), and users' e-mail addresses through
// kapi_user_directory_v1. The kernel writes the log when a node reports
// traffic; the package never writes it. Responses are byte-compatible with
// the legacy handlers (internal/tests/machinetelemetrycompat).
//
// Three routes stay bridged; the reasons are in the host's bridgedRoutes
// (packages/machine-telemetry/control/service.go): the dashboard (counts over
// five domains, the online users the kernel keeps in its cache, and a
// snapshot the kernel caches), the system information (the kernel binary's
// build metadata) and the monitoring WebSocket.
package native

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the
	// kapi_traffic_log_v1 and kapi_user_directory_v1 views are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Now defaults to time.Now. The hourly buckets are in the local time
	// zone, as the kernel's; the kernel passes its TZ to the host.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"telemetry.admin.traffic.hourly.get":       s.HourlyTraffic,
		"telemetry.admin.traffic.user_ranking.get": s.UserTrafficRanking,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

// jsonAnswer is the legacy handlers' c.JSON(code, body): encoding/json, as
// gin uses, with gin's content type.
func jsonAnswer(code int, body any) (pluginhostsdk.NativeResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, err
	}
	return pluginhostsdk.NativeResponse{
		StatusCode: uint32(code), Body: encoded, // #nosec G115 -- HTTP status codes.
		Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
	}, nil
}

// query is gin's c.Query on the forwarded query string.
func query(request pluginhostsdk.NativeRequest, key string) string {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}
