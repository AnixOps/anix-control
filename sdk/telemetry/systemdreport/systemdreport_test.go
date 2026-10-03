package systemdreport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeKeepsOnlyTheWhitelistedFields(t *testing.T) {
	report, err := Sanitize([]byte(`{
		"supported": true, "window_seconds": 600, "hostname": "dropped",
		"units": [
			{"name": "sshd.service", "active_state": "active", "sub_state": "running",
			 "cpu_avg_percent": 0.5, "cpu_peak_percent": 3.25, "memory_bytes": 1048576, "memory_peak_bytes": 2097152,
			 "pid": 42, "fragment_path": "/lib/systemd/system/sshd.service"},
			{"name": "a@b\\x2d1.service", "active_state": "failed", "sub_state": "failed"},
			{"name": "user@1000.service", "active_state": "active", "sub_state": "running"},
			{"name": "run-r1234.service", "active_state": "active", "sub_state": "running"},
			{"name": "session-3.scope", "active_state": "active", "sub_state": "running"},
			{"name": "dev-sda.device", "active_state": "active", "sub_state": "plugged"}
		]}`))
	require.NoError(t, err)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	assert.JSONEq(t, `{"supported":true,"window_seconds":600,"units":[
		{"name":"a@b\\x2d1.service","active_state":"failed","sub_state":"failed","cpu_avg_percent":0,"cpu_peak_percent":0,"memory_bytes":0,"memory_peak_bytes":0},
		{"name":"sshd.service","active_state":"active","sub_state":"running","cpu_avg_percent":0.5,"cpu_peak_percent":3.25,"memory_bytes":1048576,"memory_peak_bytes":2097152}
	]}`, string(encoded))
}

func TestSanitizeRefusesDescriptionAndExecStart(t *testing.T) {
	for _, payload := range []string{
		`{"supported": true, "window_seconds": 600, "units": [{"name": "a.service", "active_state": "active", "sub_state": "running", "description": "x"}]}`,
		`{"supported": true, "window_seconds": 600, "units": [{"name": "a.service", "active_state": "active", "sub_state": "running", "Description": "x"}]}`,
		`{"supported": true, "window_seconds": 600, "units": [{"name": "a.service", "active_state": "active", "sub_state": "running", "ExecStart": "/bin/x"}]}`,
		`{"supported": true, "window_seconds": 600, "units": [{"name": "a.service", "active_state": "active", "sub_state": "running", "exec_start": "/bin/x"}]}`,
		`{"supported": true, "window_seconds": 600, "units": [{"name": "a.service", "active_state": "active", "sub_state": "running", "exec-start": null}]}`,
		`{"supported": true, "window_seconds": 600, "description": "x", "units": []}`,
		`{"supported": false, "units": [{"name": "a.service", "ExecStart": "/bin/x"}]}`,
	} {
		_, err := Sanitize([]byte(payload))
		require.ErrorIs(t, err, ErrForbiddenField, payload)
		require.ErrorIs(t, err, ErrInvalid, payload)
	}
}

func unitJSON(name string) string {
	return fmt.Sprintf(`{"name": %q, "active_state": "active", "sub_state": "running", "cpu_avg_percent": 99.99, "cpu_peak_percent": 100, "memory_bytes": 18446744073709551615, "memory_peak_bytes": 18446744073709551615}`, name)
}

func reportJSON(units ...string) string {
	return `{"supported": true, "window_seconds": 600, "units": [` + strings.Join(units, ",") + `]}`
}

func TestSanitizeRefusesMalformedReports(t *testing.T) {
	tooMany := make([]string, MaxUnits+1)
	for index := range tooMany {
		tooMany[index] = unitJSON(fmt.Sprintf("u%d.service", index))
	}
	for name, payload := range map[string]string{
		"not an object":        `[]`,
		"null":                 `null`,
		"not JSON":             `{`,
		"no supported":         `{"window_seconds": 600, "units": []}`,
		"supported a string":   `{"supported": "yes", "window_seconds": 600}`,
		"no window":            `{"supported": true, "units": []}`,
		"window too long":      `{"supported": true, "window_seconds": 3601, "units": []}`,
		"units not a list":     `{"supported": true, "window_seconds": 600, "units": {}}`,
		"unit not an object":   reportJSON(`"a.service"`),
		"too many units":       reportJSON(tooMany...),
		"name too long":        reportJSON(unitJSON(strings.Repeat("a", MaxNameLength-len(".service")+1) + ".service")),
		"empty name":           reportJSON(unitJSON("")),
		"bad name":             reportJSON(unitJSON("a b.service")),
		"duplicate":            reportJSON(unitJSON("a.service"), unitJSON("a.service")),
		"bad state":            reportJSON(`{"name": "a.service", "active_state": "Active", "sub_state": "running"}`),
		"missing state":        reportJSON(`{"name": "a.service", "sub_state": "running"}`),
		"negative cpu":         reportJSON(`{"name": "a.service", "active_state": "active", "sub_state": "running", "cpu_avg_percent": -1}`),
		"cpu too high":         reportJSON(`{"name": "a.service", "active_state": "active", "sub_state": "running", "cpu_peak_percent": 102401}`),
		"negative memory":      reportJSON(`{"name": "a.service", "active_state": "active", "sub_state": "running", "memory_bytes": -1}`),
		"fractional memory":    reportJSON(`{"name": "a.service", "active_state": "active", "sub_state": "running", "memory_bytes": 1.5}`),
		"reason not printable": `{"supported": false, "unsupported_reason": "a\nb"}`,
		"reason too long":      `{"supported": false, "unsupported_reason": "` + strings.Repeat("a", MaxReasonLength+1) + `"}`,
	} {
		_, err := Sanitize([]byte(payload))
		require.ErrorIs(t, err, ErrInvalid, name)
	}
}

func TestSanitizeUnsupportedNodeShowsNoUnits(t *testing.T) {
	report, err := Sanitize([]byte(`{"supported": false, "unsupported_reason": "cgroup v1", "window_seconds": 99999, "units": [{"name": "a.service"}]}`))
	require.NoError(t, err)
	assert.Equal(t, Report{Supported: false, UnsupportedReason: "cgroup v1", Units: []Unit{}}, report)

	report, err = Sanitize([]byte(`{"supported": true, "unsupported_reason": "ignored", "window_seconds": 600, "units": null}`))
	require.NoError(t, err)
	assert.Equal(t, Report{Supported: true, WindowSeconds: 600, Units: []Unit{}}, report)
}

// The largest report the whitelist allows fits the generic payload cap.
func TestMaximalReportFitsThePayloadCap(t *testing.T) {
	units := make([]string, MaxUnits)
	for index := range units {
		suffix := fmt.Sprintf("-%03d.service", index)
		units[index] = fmt.Sprintf(`{"name": %q, "active_state": "deactivating", "sub_state": "stop-sigterm", "cpu_avg_percent": 102399.123456789, "cpu_peak_percent": 102399.123456789, "memory_bytes": 18446744073709551615, "memory_peak_bytes": 18446744073709551615}`,
			strings.Repeat("a", MaxNameLength-len(suffix))+suffix)
	}
	report, err := Sanitize([]byte(reportJSON(units...)))
	require.NoError(t, err)
	require.Len(t, report.Units, MaxUnits)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	require.NoError(t, agentcontrol.ValidatePackageReportPayloadSize(encoded), "%d bytes", len(encoded))
}

func TestCollectableAndSelected(t *testing.T) {
	for name, want := range map[string]bool{
		"sshd.service": true, "getty@tty1.service": true, ".service": false,
		"user@1000.service": false, "run-u12.service": false, "session-2.scope": false, "sshd.socket": false,
	} {
		assert.Equal(t, want, Collectable(name), name)
	}
	assert.True(t, Selected("nginx.service", nil, nil))
	assert.True(t, Selected("nginx.service", []string{"nginx*", "caddy*"}, nil))
	assert.False(t, Selected("sshd.service", []string{"nginx*"}, nil))
	assert.False(t, Selected("nginx.service", nil, []string{"ngin?.service"}))
	assert.False(t, Selected("nginx.service", []string{"["}, nil), "a malformed glob matches nothing")
	assert.False(t, Selected("user@0.service", []string{"*"}, nil))
}

func TestIsStale(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	assert.False(t, IsStale(now.Add(-StaleAfter), now))
	assert.True(t, IsStale(now.Add(-StaleAfter-time.Second), now))
	assert.False(t, IsStale(now.Add(time.Minute), now))
	assert.True(t, errors.Is(ErrForbiddenField, ErrInvalid))
}
