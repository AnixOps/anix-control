package agentinstall

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Readiness check ids, stable for the console (it shows its own wording per
// id and the check's message as detail).
const (
	CheckPublicURL       = "public_url"
	CheckGRPC            = "grpc"
	CheckCA              = "ca"
	CheckMTLSMode        = "mtls_mode"
	CheckAgentRelease    = "agent_release"
	CheckScriptSignature = "script_signature"
)

// Levels of a failed check. An error blocks the install command: it would
// fail on the machine. A warning does not: the command works, but the
// administrator should know.
const (
	LevelError   = "error"
	LevelWarning = "warning"
)

// defaultGRPCPort is the gRPC listener's port when grpc.port is unset
// (ResolveSettings uses the same).
const defaultGRPCPort = 50051

// ReadinessInput is what the readiness check reads from a Control's
// configuration and request. It makes no network call.
type ReadinessInput struct {
	// PublicURL is agent_install.public_url; RequestOrigin is the address
	// the administrator reached Control at, used when PublicURL is empty.
	PublicURL     string
	RequestOrigin string
	// GRPCTarget is agent_install.grpc_target.
	GRPCTarget string
	// GRPCEnabled, GRPCPort, GRPCTLSCertFile and GRPCTLSKeyFile are grpc.*.
	GRPCEnabled     bool
	GRPCPort        int
	GRPCTLSCertFile string
	GRPCTLSKeyFile  string
	// BuiltinPKI is module_runtime.pki being builtin; CAKEK is whether
	// module_runtime.ca_kek is set, and CAKEKProblem why a set key cannot
	// be used (empty when it can).
	BuiltinPKI   bool
	CAKEK        bool
	CAKEKProblem string
	// MTLSMode is agent_control.mtls, resolved (off, optional, preferred or
	// required).
	MTLSMode string
	// SignatureFile is agent_install.signature_file and OfficialPublicKey
	// plugins.official_public_key.
	SignatureFile     string
	OfficialPublicKey string
	// AgentVersion is agent_install.agent_version (empty: the Agent shares
	// Control's number), ReleaseVersion Control's version, ArtifactDir
	// agent_install.artifact_dir and CNMirrorURL agent_install.cn_mirror_url.
	AgentVersion   string
	ReleaseVersion string
	ArtifactDir    string
	CNMirrorURL    string
}

// ReadinessCheck is one check of an install command's prerequisites.
type ReadinessCheck struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// Level is what a failure means: error (the command would fail) or
	// warning (it works). It is set on a passing check too, as what its
	// failure would be.
	Level string `json:"level"`
	// Message says what Control found; Fix says what to change (empty
	// when the check passed).
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// Readiness is the answer of CheckReadiness.
type Readiness struct {
	// Ready is true when no error-level check fails: the install command
	// can work.
	Ready  bool             `json:"ready"`
	Checks []ReadinessCheck `json:"checks"`
}

// CheckReadiness tells whether the install command an administrator would
// copy can work on a machine: nodes must reach Control over https, the
// gRPC listener must be reachable with TLS, the built-in CA must be there
// (the group's join token is sealed with its key, and the node's
// certificate is signed by it), and the Agent release must be one the
// script can download. It reads the configuration and the files it names,
// and calls no network.
func CheckReadiness(in ReadinessInput) Readiness {
	checks := []ReadinessCheck{
		checkPublicURL(in),
		checkGRPC(in),
		checkCA(in),
		checkMTLSMode(in),
		checkAgentRelease(in),
		checkScriptSignature(in),
	}
	ready := true
	for _, check := range checks {
		if !check.OK && check.Level == LevelError {
			ready = false
		}
	}
	return Readiness{Ready: ready, Checks: checks}
}

// effectiveControlURL is the address nodes would use and where it came
// from.
func effectiveControlURL(in ReadinessInput) (value string, pinned bool) {
	if value = strings.TrimRight(strings.TrimSpace(in.PublicURL), "/"); value != "" {
		return value, true
	}
	return strings.TrimRight(strings.TrimSpace(in.RequestOrigin), "/"), false
}

// isLoopbackHost tells whether host is localhost or a loopback address:
// nothing a node can reach.
func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func checkPublicURL(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckPublicURL, Level: LevelError}
	fix := "Set agent_install.public_url to the https address nodes reach Control at (for example https://panel.example.com) and restart Control. Behind a reverse proxy, also list the proxy in server.trusted_proxies."
	value, pinned := effectiveControlURL(in)
	if value == "" {
		check.Message = "Control cannot tell the address nodes reach it at: agent_install.public_url is not set."
		check.Fix = fix
		return check
	}
	parsed, err := url.Parse(value)
	switch {
	case err != nil || parsed.Host == "":
		check.Message = fmt.Sprintf("%q is not an address nodes can use.", value)
		check.Fix = fix
	case parsed.Scheme != "https":
		check.Message = fmt.Sprintf("Nodes would use %s, which is not https: the install script and the join refuse plain http.", value)
		check.Fix = fix
	case isLoopbackHost(parsed.Hostname()):
		check.Message = fmt.Sprintf("Nodes would use %s, a loopback address that other machines cannot reach.", value)
		check.Fix = fix
	case pinned:
		check.OK = true
		check.Message = "Nodes reach Control at " + value + "."
	default:
		check.OK = true
		check.Message = "Nodes reach Control at " + value + ", the address this page was opened with. Set agent_install.public_url to pin it."
	}
	return check
}

// grpcTarget resolves the host:port nodes dial for the gRPC listener, as
// ResolveSettings does, without resolving the name.
func grpcTarget(in ReadinessInput) string {
	if target := strings.TrimSpace(in.GRPCTarget); target != "" {
		return target
	}
	value, _ := effectiveControlURL(in)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	port := in.GRPCPort
	if port <= 0 {
		port = defaultGRPCPort
	}
	return net.JoinHostPort(parsed.Hostname(), strconv.Itoa(port))
}

func checkGRPC(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckGRPC, Level: LevelError}
	var problems, fixes []string
	target := grpcTarget(in)
	switch host, port, err := net.SplitHostPort(target); {
	case target == "":
		problems = append(problems, "Control cannot work out the gRPC address nodes dial.")
		fixes = append(fixes, "Set agent_install.grpc_target to host:port of the gRPC listener, or fix agent_install.public_url.")
	case err != nil || host == "" || port == "":
		problems = append(problems, fmt.Sprintf("The gRPC target %q is not host:port.", target))
		fixes = append(fixes, "Set agent_install.grpc_target to host:port of the gRPC listener.")
	case isLoopbackHost(host):
		problems = append(problems, fmt.Sprintf("The gRPC target %s is a loopback address that nodes cannot reach.", target))
		fixes = append(fixes, "Set agent_install.grpc_target to the public host:port of the gRPC listener.")
	}
	if !in.GRPCEnabled {
		problems = append(problems, "The gRPC listener is off: nodes join and stay connected there.")
		fixes = append(fixes, "Set grpc.enabled: true and restart Control.")
	}
	if strings.TrimSpace(in.GRPCTLSCertFile) == "" || strings.TrimSpace(in.GRPCTLSKeyFile) == "" {
		problems = append(problems, "The gRPC listener has no TLS certificate: nodes verify it and present their own certificate.")
		fixes = append(fixes, "Set grpc.tls_cert_file and grpc.tls_key_file to a certificate for the gRPC host that nodes trust (a publicly trusted one), then restart Control.")
	}
	if len(problems) == 0 {
		check.OK = true
		check.Message = "Nodes dial the gRPC listener at " + target + " over TLS."
		return check
	}
	check.Message = strings.Join(problems, " ")
	check.Fix = strings.Join(fixes, " ")
	return check
}

func checkCA(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckCA, Level: LevelError}
	const fix = "Set module_runtime.ca_kek (ANIX_CONTROL_MODULE_RUNTIME_CA_KEK) to a 32-byte key (base64 or hex) and restart Control. Generate one with: openssl rand -base64 32"
	switch {
	case !in.BuiltinPKI:
		check.Message = "module_runtime.pki is external: Control holds no CA key to sign node certificates or to seal join tokens."
		check.Fix = "Set module_runtime.pki to builtin and module_runtime.ca_kek to a 32-byte key, then restart Control."
	case !in.CAKEK:
		check.Message = "module_runtime.ca_kek is not set: node certificates are signed by the built-in CA, and group join tokens are sealed with its key."
		check.Fix = fix
	case in.CAKEKProblem != "":
		check.Message = "module_runtime.ca_kek cannot be used: " + in.CAKEKProblem
		check.Fix = fix
	default:
		check.OK = true
		check.Message = "The built-in agent CA is on; join tokens are sealed with module_runtime.ca_kek."
	}
	return check
}

func checkMTLSMode(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckMTLSMode, Level: LevelError}
	mode := strings.ToLower(strings.TrimSpace(in.MTLSMode))
	if mode == "" {
		mode = "required"
	}
	if mode == "off" {
		check.Message = "agent_control.mtls is off: agent enrollment is unavailable, so a node cannot join."
		check.Fix = "Remove agent_control.mtls (required is the default) or set it to required, then restart Control."
		return check
	}
	check.OK = true
	check.Message = "agent_control.mtls is " + mode + ": nodes join and connect with their certificate."
	return check
}

// agentVersion is the Agent release the command installs, and whether it
// is pinned by agent_install.agent_version.
func agentVersion(in ReadinessInput) (version string, pinned bool) {
	if version = strings.TrimSpace(in.AgentVersion); version != "" {
		return version, true
	}
	control := strings.TrimSpace(in.ReleaseVersion)
	if control == "" {
		return "", false
	}
	return "v" + strings.TrimPrefix(control, "v"), false
}

func checkAgentRelease(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckAgentRelease, Level: LevelError}
	version, pinned := agentVersion(in)
	if !ReleaseTagPattern.MatchString(version) {
		check.Message = "This Control is not a release build, so it does not know which Agent release to install."
		check.Fix = "Set agent_install.agent_version to the Agent release tag (for example v4.2.0) and restart Control."
		return check
	}
	s := Settings{AgentVersion: version, ArtifactDir: in.ArtifactDir, CNMirrorURL: in.CNMirrorURL}
	switch {
	case len(s.localDigests()) == len(Assets):
		check.OK = true
		check.Message = "Agent " + version + " is served by this Control from agent_install.artifact_dir."
	case strings.TrimSpace(in.CNMirrorURL) != "":
		check.OK = true
		check.Message = "Agent " + version + " is downloaded from GitHub releases, or from the mainland mirror with --mirror cn."
	case strings.TrimSpace(in.ArtifactDir) != "":
		check.Level = LevelWarning
		check.Message = fmt.Sprintf("agent_install.artifact_dir is set but does not hold Agent %s (both architectures with their .dgst files); nodes download it from GitHub releases instead.", version)
		check.Fix = fmt.Sprintf("Put the Agent release into %s/%s/ (anix-agent-linux-64.zip, anix-agent-linux-arm64-v8a.zip and their .dgst files), or leave agent_install.artifact_dir empty.", strings.TrimRight(strings.TrimSpace(in.ArtifactDir), "/"), version)
	case pinned:
		check.OK = true
		check.Message = "Agent " + version + " is downloaded from GitHub releases."
	default:
		check.OK = true
		check.Message = fmt.Sprintf("Agent %s (Control's own version) is downloaded from GitHub releases; Control cannot check from here that the release exists.", version)
	}
	return check
}

func checkScriptSignature(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckScriptSignature, Level: LevelWarning}
	path := strings.TrimSpace(in.SignatureFile)
	if path == "" {
		check.OK = true
		check.Message = "No release signature is configured for the install script (agent_install.signature_file); it is served unsigned."
		return check
	}
	if _, err := LoadSignature(path, in.OfficialPublicKey); err != nil {
		check.Message = "The configured install script signature does not verify: " + err.Error()
		check.Fix = "Point agent_install.signature_file at the agent-install.sh.sig release asset of this Control's version and check plugins.official_public_key, or leave agent_install.signature_file empty."
		return check
	}
	check.OK = true
	check.Message = "The install script's release signature verifies; it is served at /install.sh.sig."
	return check
}
