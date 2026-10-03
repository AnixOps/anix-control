// Package systemdreport is the schema of the systemd.services package
// report: the per-node systemd services table of the machine-telemetry
// package (docs/architecture/package-reports.md). The Agent plugin builds a
// Report and filters unit names with Collectable and Selected; Control runs
// every payload through Sanitize before it stores it, so only the fields
// below ever reach the database, whatever an Agent sends.
//
// A unit carries its name, ActiveState, SubState, its CPU use as the average
// and the peak over the window, and its current and peak memory: nothing
// else. Description and ExecStart are never sent; a payload that has either
// is refused.
package systemdreport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// Kind is the PackageReport kind of the services table.
	Kind = "systemd.services"
	// Capability is the manifest capability a release of the reporting
	// package must declare for Control to accept the report.
	Capability = "telemetry.systemd.read"
	// MaxUnits caps the units of one report.
	MaxUnits = 512
	// MaxNameLength caps a unit name, in bytes.
	MaxNameLength = 256
	// MaxStateLength caps active_state and sub_state.
	MaxStateLength = 32
	// MaxReasonLength caps unsupported_reason.
	MaxReasonLength = 128
	// WindowSeconds is the averaging window the collector uses: 10 minutes.
	WindowSeconds = 600
	// MaxWindowSeconds caps window_seconds.
	MaxWindowSeconds = 3600
	// MaxCPUPercent caps a CPU figure, in percent of one CPU (1024 CPUs).
	MaxCPUPercent = 102400
	// StaleAfter is how old a report may be before it reads as stale.
	StaleAfter = 25 * time.Minute
)

// Report is the payload_json of a systemd.services PackageReport.
type Report struct {
	// Supported is false on a node without systemd or cgroup v2; Units is
	// then empty and UnsupportedReason says why.
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupported_reason,omitempty"`
	// WindowSeconds is the window of the averages and peaks.
	WindowSeconds int    `json:"window_seconds"`
	Units         []Unit `json:"units"`
}

// Unit is one .service unit.
type Unit struct {
	Name        string `json:"name"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	// CPUAvgPercent and CPUPeakPercent are the unit's CPU use over the
	// window, in percent of one CPU.
	CPUAvgPercent  float64 `json:"cpu_avg_percent"`
	CPUPeakPercent float64 `json:"cpu_peak_percent"`
	// MemoryBytes is the unit's current memory, MemoryPeakBytes its peak.
	MemoryBytes     uint64 `json:"memory_bytes"`
	MemoryPeakBytes uint64 `json:"memory_peak_bytes"`
}

var (
	// ErrInvalid wraps every reason Sanitize refuses a payload.
	ErrInvalid = errors.New("invalid systemd.services report")
	// ErrForbiddenField means the payload has a Description or ExecStart
	// field, which a report must never carry.
	ErrForbiddenField = fmt.Errorf("%w: description and exec_start must never be sent", ErrInvalid)
)

var (
	// unitNamePattern is systemd's unit name alphabet.
	unitNamePattern = regexp.MustCompile(`^[A-Za-z0-9:_.\\@-]+$`)
	statePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// IsStale reports whether a report observed at observedAt is stale at now.
func IsStale(observedAt, now time.Time) bool {
	return now.Sub(observedAt) > StaleAfter
}

// Collectable reports whether a unit may be reported at all: a .service
// unit that is not a user manager (user@*) or a transient run-* unit. Scope
// units, session-*.scope among them, are not services.
func Collectable(name string) bool {
	return strings.HasSuffix(name, ".service") && len(name) > len(".service") &&
		!strings.HasPrefix(name, "user@") && !strings.HasPrefix(name, "run-")
}

// Selected applies a node's include and exclude globs (path.Match syntax)
// to a collectable unit: with includes, the name must match one; it must
// match no exclude. A malformed glob matches nothing.
func Selected(name string, include, exclude []string) bool {
	if !Collectable(name) {
		return false
	}
	if len(include) > 0 && !matchesAny(name, include) {
		return false
	}
	return !matchesAny(name, exclude)
}

func matchesAny(name string, globs []string) bool {
	for _, glob := range globs {
		if matched, err := path.Match(glob, name); err == nil && matched {
			return true
		}
	}
	return false
}

// Sanitize parses a payload and returns the report Control may store: the
// whitelisted fields only, units sorted by name. Unknown fields and units
// that are not Collectable are dropped. It refuses a payload that is not a
// JSON object, has a Description or ExecStart field at any level, has more
// than MaxUnits units, a duplicate unit, a name over MaxNameLength bytes or
// outside the unit name alphabet, a malformed state or a number out of
// range.
func Sanitize(payload []byte) (Report, error) {
	fields, err := decodeObject(payload, "report")
	if err != nil {
		return Report{}, err
	}
	var report Report
	var supported *bool
	if err := decodeField(fields, "supported", &supported); err != nil {
		return Report{}, err
	}
	if supported == nil {
		return Report{}, fmt.Errorf("%w: supported is required", ErrInvalid)
	}
	report.Supported = *supported
	if err := decodeField(fields, "unsupported_reason", &report.UnsupportedReason); err != nil {
		return Report{}, err
	}
	if err := decodeField(fields, "window_seconds", &report.WindowSeconds); err != nil {
		return Report{}, err
	}
	var rawUnits []json.RawMessage
	if err := decodeField(fields, "units", &rawUnits); err != nil {
		return Report{}, err
	}
	if len(rawUnits) > MaxUnits {
		return Report{}, fmt.Errorf("%w: more than %d units", ErrInvalid, MaxUnits)
	}

	if !report.Supported {
		if len(report.UnsupportedReason) > MaxReasonLength || !printableASCII(report.UnsupportedReason) {
			return Report{}, fmt.Errorf("%w: unsupported_reason is invalid", ErrInvalid)
		}
		// Nothing to show for an unsupported node, whatever was sent.
		if report.WindowSeconds < 0 || report.WindowSeconds > MaxWindowSeconds {
			report.WindowSeconds = 0
		}
		report.Units = []Unit{}
		return report, checkUnitsForbidden(rawUnits)
	}
	report.UnsupportedReason = ""
	if report.WindowSeconds <= 0 || report.WindowSeconds > MaxWindowSeconds {
		return Report{}, fmt.Errorf("%w: window_seconds must be 1 to %d", ErrInvalid, MaxWindowSeconds)
	}

	report.Units = make([]Unit, 0, len(rawUnits))
	seen := make(map[string]struct{}, len(rawUnits))
	for index, raw := range rawUnits {
		unit, err := sanitizeUnit(raw, index)
		if err != nil {
			return Report{}, err
		}
		if _, duplicate := seen[unit.Name]; duplicate {
			return Report{}, fmt.Errorf("%w: unit %d: %q is listed twice", ErrInvalid, index, unit.Name)
		}
		seen[unit.Name] = struct{}{}
		if !Collectable(unit.Name) {
			continue
		}
		report.Units = append(report.Units, unit)
	}
	sort.Slice(report.Units, func(left, right int) bool { return report.Units[left].Name < report.Units[right].Name })
	return report, nil
}

func checkUnitsForbidden(rawUnits []json.RawMessage) error {
	for index, raw := range rawUnits {
		if _, err := decodeObject(raw, fmt.Sprintf("unit %d", index)); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeUnit(raw json.RawMessage, index int) (Unit, error) {
	where := fmt.Sprintf("unit %d", index)
	fields, err := decodeObject(raw, where)
	if err != nil {
		return Unit{}, err
	}
	var unit Unit
	for _, field := range []struct {
		name   string
		target any
	}{
		{"name", &unit.Name}, {"active_state", &unit.ActiveState}, {"sub_state", &unit.SubState},
		{"cpu_avg_percent", &unit.CPUAvgPercent}, {"cpu_peak_percent", &unit.CPUPeakPercent},
		{"memory_bytes", &unit.MemoryBytes}, {"memory_peak_bytes", &unit.MemoryPeakBytes},
	} {
		if err := decodeField(fields, field.name, field.target); err != nil {
			return Unit{}, fmt.Errorf("%s: %w", where, err)
		}
	}
	if unit.Name == "" || len(unit.Name) > MaxNameLength || !unitNamePattern.MatchString(unit.Name) {
		return Unit{}, fmt.Errorf("%w: %s: name is empty, longer than %d bytes or not a unit name", ErrInvalid, where, MaxNameLength)
	}
	for _, state := range []string{unit.ActiveState, unit.SubState} {
		if len(state) > MaxStateLength || !statePattern.MatchString(state) {
			return Unit{}, fmt.Errorf("%w: %s: active_state and sub_state must be lowercase words of at most %d bytes", ErrInvalid, where, MaxStateLength)
		}
	}
	for _, percent := range []float64{unit.CPUAvgPercent, unit.CPUPeakPercent} {
		if math.IsNaN(percent) || percent < 0 || percent > MaxCPUPercent {
			return Unit{}, fmt.Errorf("%w: %s: CPU percent out of range", ErrInvalid, where)
		}
	}
	return unit, nil
}

// decodeObject decodes a JSON object into its fields and refuses one with a
// forbidden field.
func decodeObject(raw []byte, where string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("%w: %s is not a JSON object", ErrInvalid, where)
	}
	for key := range fields {
		if forbiddenKey(key) {
			return nil, fmt.Errorf("%w (%s has %q)", ErrForbiddenField, where, key)
		}
	}
	return fields, nil
}

// forbiddenKey matches Description and ExecStart in any spelling:
// description, Description, exec_start, ExecStart, exec-start.
func forbiddenKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
	return normalized == "description" || normalized == "execstart"
}

// decodeField decodes one field, if present and not null, into target.
func decodeField(fields map[string]json.RawMessage, name string, target any) error {
	raw, ok := fields[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: %s has the wrong type or is out of range", ErrInvalid, name)
	}
	return nil
}

func printableASCII(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] > 0x7e {
			return false
		}
	}
	return true
}
