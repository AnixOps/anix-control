package nodesecrets

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Fallback reasons: why a dual_read reader used the legacy column.
const (
	// FallbackMissing: the new table holds no current row for the secret.
	FallbackMissing = "missing"
	// FallbackMismatch: the new table's row differs from the legacy column.
	FallbackMismatch = "mismatch"
	// FallbackError: the new table could not be read.
	FallbackError = "error"
)

// loggedSubjectsLimit bounds the subjects this process remembers having
// logged. Past it, the counters still count and nothing more is logged.
const loggedSubjectsLimit = 10000

// counterSet is a set of monotonic counters keyed by their label values.
type counterSet struct {
	values sync.Map // [3]string -> *atomic.Uint64
}

func (s *counterSet) add(labels [3]string) {
	value, _ := s.values.LoadOrStore(labels, new(atomic.Uint64))
	value.(*atomic.Uint64).Add(1)
}

func (s *counterSet) get(match func(labels [3]string) bool) uint64 {
	var total uint64
	s.values.Range(func(key, value any) bool {
		if match(key.([3]string)) {
			total += value.(*atomic.Uint64).Load()
		}
		return true
	})
	return total
}

func (s *counterSet) write(body *strings.Builder, name string, labelNames [3]string) {
	type sample struct {
		labels [3]string
		value  uint64
	}
	var samples []sample
	s.values.Range(func(key, value any) bool {
		samples = append(samples, sample{labels: key.([3]string), value: value.(*atomic.Uint64).Load()})
		return true
	})
	sort.Slice(samples, func(i, j int) bool {
		a, b := samples[i].labels, samples[j].labels
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		return a[2] < b[2]
	})
	for _, sample := range samples {
		body.WriteString(name + "{")
		for i, label := range labelNames {
			if i > 0 {
				body.WriteString(",")
			}
			body.WriteString(label + "=\"" + sample.labels[i] + "\"")
		}
		body.WriteString("} " + formatCount(sample.value) + "\n")
	}
}

var (
	fallbacks counterSet // table, kind, reason
	invalids  counterSet // table, type, reason

	loggedMu       sync.Mutex
	loggedSubjects = map[string]struct{}{}
	loggedFull     bool
)

// firstLog reports whether subject is logged for the first time in this
// process. Readers log once per subject, so a node polling every minute
// does not repeat the line.
func firstLog(subject string) bool {
	loggedMu.Lock()
	defer loggedMu.Unlock()
	if _, seen := loggedSubjects[subject]; seen {
		return false
	}
	if len(loggedSubjects) >= loggedSubjectsLimit {
		if !loggedFull {
			loggedFull = true
			slog.Warn("node secret reports: too many subjects to log each once; only the counters continue",
				"component", "nodesecrets", "limit", loggedSubjectsLimit)
		}
		return false
	}
	loggedSubjects[subject] = struct{}{}
	return true
}

// ForgetLogged forgets which subjects this process has logged, so the next
// fallback or finding of each is logged again. Control never calls it;
// tests that reuse row ids across databases do.
func ForgetLogged() {
	loggedMu.Lock()
	defer loggedMu.Unlock()
	loggedSubjects = map[string]struct{}{}
	loggedFull = false
}

// recordFallback counts a dual_read reader that used the legacy column, and
// logs it once per subject. subject names the row and position, never a
// value.
func recordFallback(table, kind, reason, subject string) {
	fallbacks.add([3]string{table, kind, reason})
	if firstLog("fallback " + table + " " + kind + " " + subject) {
		slog.Warn("node secret read fell back to the legacy column",
			"component", "nodesecrets", "table", table, "kind", kind, "subject", subject, "reason", reason)
	}
}

// FallbackCount answers how many reads of table's secrets of kind fell back
// to the legacy column in this process; an empty reason counts every
// reason.
func FallbackCount(table, kind, reason string) uint64 {
	return fallbacks.get(func(labels [3]string) bool {
		return labels[0] == table && labels[1] == kind && (reason == "" || labels[2] == reason)
	})
}

// RecordInvalid counts a node configuration build that used a row whose
// secret fails validation (validate on build, report-only: decision D7),
// and logs it once per subject. The row is not left out. subject names the
// node, protocol and field, never a value; protocolType and reason are
// bounded label values.
func RecordInvalid(table, protocolType, reason, subject string) {
	invalids.add([3]string{table, labelValue(protocolType), labelValue(reason)})
	if firstLog("invalid " + table + " " + subject + " " + reason) {
		slog.Warn("node configuration uses a secret that fails validation; reported, not excluded",
			"component", "nodesecrets", "table", table, "subject", subject, "reason", reason)
	}
}

// InvalidCount answers how many builds of table's rows used a secret that
// fails validation for reason, in this process; an empty reason counts every
// reason.
func InvalidCount(table, reason string) uint64 {
	return invalids.get(func(labels [3]string) bool {
		return labels[0] == table && (reason == "" || labels[2] == reason)
	})
}

// labelValue keeps a label value bounded and safe in the text format:
// lower-case letters, digits, '_' and '-', at most 32 of them.
func labelValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 32 {
		return "other"
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return "other"
		}
	}
	return value
}

func formatCount(value uint64) string {
	var buffer [20]byte
	i := len(buffer)
	for value >= 10 {
		i--
		buffer[i] = byte(value%10) + '0'
		value /= 10
	}
	i--
	buffer[i] = byte(value) + '0'
	return string(buffer[i:])
}

// WritePrometheus writes the dual_read fallback and the validate-on-build
// counters of this process.
func WritePrometheus(body *strings.Builder) {
	body.WriteString("# HELP anixops_node_secrets_fallback_total Node secret reads in phase dual_read that used the legacy column (section 4.3).\n")
	body.WriteString("# TYPE anixops_node_secrets_fallback_total counter\n")
	fallbacks.write(body, "anixops_node_secrets_fallback_total", [3]string{"table", "kind", "reason"})
	body.WriteString("# HELP anixops_node_secrets_invalid_total Node configuration builds that used a secret failing validation; reported, not excluded (D7).\n")
	body.WriteString("# TYPE anixops_node_secrets_invalid_total counter\n")
	invalids.write(body, "anixops_node_secrets_invalid_total", [3]string{"table", "type", "reason"})
}
