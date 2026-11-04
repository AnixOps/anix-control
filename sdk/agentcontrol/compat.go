package agentcontrol

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Agent protocol revisions and version compatibility
// (docs/reference/agent-versioning.md).
//
// Since v5.0.0 Control and the Agent are released separately: they share
// the major and minor version (the version line, 5.0) and number their
// patches independently. What decides whether a Control and an Agent work
// together is the protocol revision: a whole number inside anix.agent.v1
// that grows by one whenever the contract gains something a side relies on
// (a message, a field or a capability the other side must serve). The
// Agent names the revision it speaks in a Hello label; Control serves a
// range of revisions and judges every session against it.
const (
	// LabelProtocolRevision is the Hello label in which an Agent names the
	// protocol revision it speaks (a decimal number). An Agent that
	// predates the label speaks ProtocolRevisionBaseline.
	LabelProtocolRevision = "protocol.revision"
	// LabelKernelMinVersion is the Hello label in which an Agent names the
	// oldest anx-node release it works with (KernelVersionPattern); an
	// Agent that predates the label names none.
	LabelKernelMinVersion = "kernel.anx-node.min"

	// ProtocolRevisionBaseline is the revision of the v5.0.0 contract:
	// every Agent released before LabelProtocolRevision speaks it.
	ProtocolRevisionBaseline uint32 = 1
	// ProtocolRevisionCurrent is the newest revision this SDK defines.
	// Raise it, and add a line to the revision table of
	// docs/reference/agent-versioning.md, with the contract change that
	// introduces it.
	ProtocolRevisionCurrent uint32 = 1
)

// Compatibility statuses, from best to worst.
const (
	// CompatibilityOK: the Agent and Control work together.
	CompatibilityOK = "compatible"
	// CompatibilityWarning: they work together, but something should be
	// upgraded (Control is older than the Agent, or the version lines
	// differ).
	CompatibilityWarning = "warning"
	// CompatibilityIncompatible: the Agent, or its kernel, is older than
	// Control or the Agent supports; features fail until it is upgraded.
	CompatibilityIncompatible = "incompatible"
)

// Compatibility problem codes.
const (
	// ProblemAgentProtocolTooOld: the Agent speaks a revision below the
	// oldest Control serves. Upgrade the Agent.
	ProblemAgentProtocolTooOld = "agent_protocol_too_old"
	// ProblemAgentProtocolTooNew: the Agent speaks a revision above the
	// newest Control serves; the newer features stay off. Upgrade Control.
	ProblemAgentProtocolTooNew = "agent_protocol_too_new"
	// ProblemVersionLineMismatch: the Agent's major.minor is not Control's.
	ProblemVersionLineMismatch = "version_line_mismatch"
	// ProblemKernelTooOld: the anx-node release is older than the Agent
	// needs. Upgrade the kernel.
	ProblemKernelTooOld = "kernel_too_old"
)

// CompatibilityProblem is one reason a session is not CompatibilityOK.
type CompatibilityProblem struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Compatibility is the judgement of one Agent session.
type Compatibility struct {
	// Status is the worst severity of Problems, CompatibilityOK without any.
	Status   string                 `json:"status"`
	Problems []CompatibilityProblem `json:"problems"`
}

// CompatibilityInput is what CheckCompatibility judges.
type CompatibilityInput struct {
	// ControlVersion is Control's release (v5.0.1); "" skips the version
	// line check.
	ControlVersion string
	// MinRevision and MaxRevision are the protocol revisions Control
	// serves.
	MinRevision uint32
	MaxRevision uint32
	// AgentVersion is the Agent's release from its Hello.
	AgentVersion string
	// AgentRevision is HelloProtocolRevision of its labels.
	AgentRevision uint32
	// KernelVersion and KernelMinVersion are the anx-node release the
	// Agent runs and the oldest it works with; either "" skips the check.
	KernelVersion    string
	KernelMinVersion string
}

var protocolRevisionPattern = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)

// HelloProtocolRevision returns the protocol revision a Hello's labels name
// (LabelProtocolRevision): ProtocolRevisionBaseline when they name none (an
// Agent released before the label) or a malformed one.
func HelloProtocolRevision(labels map[string]string) uint32 {
	value := strings.TrimSpace(labels[LabelProtocolRevision])
	if !protocolRevisionPattern.MatchString(value) {
		return ProtocolRevisionBaseline
	}
	revision, err := strconv.ParseUint(value, 10, 32)
	if err != nil || revision == 0 {
		return ProtocolRevisionBaseline
	}
	return uint32(revision)
}

// HelloKernelMinVersion returns the oldest anx-node release a Hello's
// labels say the Agent works with (LabelKernelMinVersion), "" when they
// name none or a malformed one.
func HelloKernelMinVersion(labels map[string]string) string {
	version := strings.TrimSpace(labels[LabelKernelMinVersion])
	if !KernelVersionPattern.MatchString(version) {
		return ""
	}
	return version
}

var versionLinePattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// VersionLine returns the major.minor of a release version (v5.0.1 and
// 5.0.0-rc.29.hf.1 are both "5.0"); ok is false when version is not one.
func VersionLine(version string) (line string, ok bool) {
	match := versionLinePattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return "", false
	}
	major, errMajor := strconv.Atoi(match[1])
	minor, errMinor := strconv.Atoi(match[2])
	if errMajor != nil || errMinor != nil {
		return "", false
	}
	return fmt.Sprintf("%d.%d", major, minor), true
}

// SameVersionLine reports whether two release versions share major and
// minor. Either one unparsable is not the same line.
func SameVersionLine(a, b string) bool {
	lineA, okA := VersionLine(a)
	lineB, okB := VersionLine(b)
	return okA && okB && lineA == lineB
}

// CompareKernelVersions orders two anx-node release versions
// (KernelVersionPattern, alpha < beta < rc < the release): -1, 0 or 1; ok is
// false when either is not one.
func CompareKernelVersions(a, b string) (result int, ok bool) {
	pa, okA := parseKernelVersion(a)
	pb, okB := parseKernelVersion(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range pa {
		switch {
		case pa[i] < pb[i]:
			return -1, true
		case pa[i] > pb[i]:
			return 1, true
		}
	}
	return 0, true
}

func parseKernelVersion(version string) ([5]int, bool) {
	var parts [5]int
	version = strings.TrimSpace(version)
	if !KernelVersionPattern.MatchString(version) {
		return parts, false
	}
	core, pre, _ := strings.Cut(version, "-")
	numbers := strings.Split(core, ".")
	for i, number := range numbers {
		n, err := strconv.Atoi(number)
		if err != nil {
			return parts, false
		}
		parts[i] = n
	}
	parts[3] = 3
	if pre != "" {
		kind, number, _ := strings.Cut(pre, ".")
		parts[3] = map[string]int{"alpha": 0, "beta": 1, "rc": 2}[kind]
		n, err := strconv.Atoi(number)
		if err != nil {
			return parts, false
		}
		parts[4] = n
	}
	return parts, true
}

// CheckCompatibility judges an Agent session against the Control it is
// connected to.
func CheckCompatibility(in CompatibilityInput) Compatibility {
	result := Compatibility{Status: CompatibilityOK, Problems: []CompatibilityProblem{}}
	add := func(code, severity, format string, args ...any) {
		result.Problems = append(result.Problems, CompatibilityProblem{Code: code, Severity: severity, Message: fmt.Sprintf(format, args...)})
		if severity == CompatibilityIncompatible || result.Status == CompatibilityOK {
			result.Status = severity
		}
	}
	revision := in.AgentRevision
	if revision == 0 {
		revision = ProtocolRevisionBaseline
	}
	switch {
	case in.MinRevision > 0 && revision < in.MinRevision:
		add(ProblemAgentProtocolTooOld, CompatibilityIncompatible,
			"the Agent speaks protocol revision %d, older than the oldest this Control serves (%d): upgrade the Agent", revision, in.MinRevision)
	case in.MaxRevision > 0 && revision > in.MaxRevision:
		add(ProblemAgentProtocolTooNew, CompatibilityWarning,
			"the Agent speaks protocol revision %d, newer than this Control serves (%d): its newer features stay off until Control is upgraded", revision, in.MaxRevision)
	}
	if strings.TrimSpace(in.ControlVersion) != "" && strings.TrimSpace(in.AgentVersion) != "" {
		controlLine, okControl := VersionLine(in.ControlVersion)
		agentLine, okAgent := VersionLine(in.AgentVersion)
		if okControl && okAgent && controlLine != agentLine {
			add(ProblemVersionLineMismatch, CompatibilityWarning,
				"the Agent is on version line %s and Control on %s: upgrade the Agent to a %s release", agentLine, controlLine, controlLine)
		}
	}
	if order, ok := CompareKernelVersions(in.KernelVersion, in.KernelMinVersion); ok && order < 0 {
		add(ProblemKernelTooOld, CompatibilityIncompatible,
			"anx-node %s is older than the %s this Agent needs: upgrade the kernel", in.KernelVersion, in.KernelMinVersion)
	}
	return result
}
