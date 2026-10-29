package agentinstall

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Readiness check ids, stable for the console (it shows its own wording per
// id and the check's message as detail).
const (
	CheckPublicURL        = "public_url"
	CheckPanelCertificate = "panel_certificate"
	CheckGRPC             = "grpc"
	CheckGRPCCertificate  = "grpc_certificate"
	CheckCA               = "ca"
	CheckMTLSMode         = "mtls_mode"
	CheckAgentRelease     = "agent_release"
	CheckScriptSignature  = "script_signature"
)

// Certificate states (internal/panelcert).
const (
	CertificatePending = "pending"
	CertificateValid   = "valid"
	CertificateFailed  = "failed"
)

// CertificateFacts is what the readiness check knows of a certificate
// Control serves (internal/panelcert's status): nodes download the script
// over the panel's https and verify the gRPC listener's certificate against
// their system CAs, so either must be valid and publicly trusted.
type CertificateFacts struct {
	// Name is the name the certificate is for, Names the names the served
	// certificate covers.
	Name  string
	Names []string
	// State is pending, valid or failed; Serving what handshakes get (acme,
	// file, self_signed).
	State           string
	Serving         string
	PubliclyTrusted bool
	NotAfter        time.Time
	// Error and Fix are the certificate status's last error and its fix.
	Error string
	Fix   string
}

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
	// GRPCTarget is the gRPC target nodes dial: agent_install.grpc_target,
	// else grpc.public_name (or domain) and grpc.port.
	GRPCTarget string
	// GRPCEnabled and GRPCPort are the gRPC listener's (on with domain or
	// grpc.enabled).
	GRPCEnabled bool
	GRPCPort    int
	// GRPCEdge tells whether a reverse proxy named by grpc.edge_proxies
	// terminates the nodes' TLS: its certificate is checked there, not here.
	GRPCEdge bool
	// PanelCertificate is the web side's certificate when Control
	// terminates https itself (tls.mode acme, files); nil when a CDN or
	// proxy does (external). GRPCCertificate is the gRPC listener's; nil
	// when the listener has no TLS.
	PanelCertificate *CertificateFacts
	GRPCCertificate  *CertificateFacts
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
	// AgentVersion is agent_install.agent_version, PinnedAgentVersion the
	// Agent release this Control was built with (both empty: the Agent
	// shares Control's number), ReleaseVersion Control's version,
	// ArtifactDir agent_install.artifact_dir and CNMirrorURL
	// agent_install.cn_mirror_url.
	AgentVersion       string
	PinnedAgentVersion string
	ReleaseVersion     string
	ArtifactDir        string
	CNMirrorURL        string
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
// copy can work on a machine: nodes must reach Control over https with a
// certificate they trust, the gRPC listener must be reachable with a
// publicly trusted certificate (Agents verify it against their system CAs
// and fail closed), the built-in CA must be there (the group's join token
// is sealed with its key, and the node's certificate is signed by it), and
// the Agent release must be one the script can download. It reads the
// configuration, the certificate status and the files it names, and calls
// no network.
func CheckReadiness(in ReadinessInput) Readiness {
	checks := []ReadinessCheck{
		checkPublicURL(in),
		checkPanelCertificate(in),
		checkGRPC(in),
		checkGRPCCertificate(in),
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
	fix := "Set domain to the panel's name (nodes then use https://<domain>), or agent_install.public_url to the https address nodes reach Control at, and restart Control. Behind a reverse proxy, also list the proxy in server.trusted_proxies."
	value, pinned := effectiveControlURL(in)
	if value == "" {
		check.Message = "Control cannot tell the address nodes reach it at: neither domain nor agent_install.public_url is set."
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
		check.Message = "Nodes reach Control at " + value + ", the address this page was opened with. Set domain to pin it."
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
		fixes = append(fixes, "Set domain (it turns the gRPC listener on), or grpc.enabled: true, and restart Control.")
	}
	if len(problems) == 0 {
		check.OK = true
		check.Message = "Nodes dial the gRPC listener at " + target + "."
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

// agentVersion is the Agent release the command installs, as
// ResolveSettings picks it, and whether it is pinned (by
// agent_install.agent_version or by the build).
func agentVersion(in ReadinessInput) (version string, pinned bool) {
	if version = strings.TrimSpace(in.AgentVersion); version != "" {
		return version, true
	}
	if version = strings.TrimSpace(in.PinnedAgentVersion); version != "" {
		return "v" + strings.TrimPrefix(version, "v"), true
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

// covers tells whether a certificate's names cover host (one wildcard
// label at most, as TLS clients match).
func covers(names []string, host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, name := range names {
		name = strings.TrimSuffix(strings.ToLower(name), ".")
		if name == host {
			return true
		}
		if suffix, ok := strings.CutPrefix(name, "*."); ok {
			if label, rest, found := strings.Cut(host, "."); found && label != "" && rest == suffix {
				return true
			}
		}
	}
	return false
}

// certificateProblem describes a served certificate that nodes would
// refuse, empty when it is fine.
func certificateProblem(what string, facts *CertificateFacts) (problem, fix string) {
	name := facts.Name
	switch {
	case facts.State == CertificatePending:
		return fmt.Sprintf("Control is still requesting the %s certificate for %s; until it has one it serves a self-signed certificate, which nodes refuse.", what, name),
			"Wait a minute and check again. If it stays pending, run anix-control tls status on the server."
	case facts.State != CertificateValid:
		problem = fmt.Sprintf("The %s certificate for %s is not usable, so Control serves a self-signed one, which nodes refuse", what, name)
		if facts.Error != "" {
			problem += ": " + facts.Error
		}
		fix = facts.Fix
		if fix == "" {
			fix = "See anix-control tls status on the server; anix-control tls retry tries again."
		}
		return problem + ".", fix
	case !facts.PubliclyTrusted:
		problem = fmt.Sprintf("The %s certificate for %s does not verify against the system CAs, and nodes accept only publicly trusted certificates", what, name)
		if facts.Error != "" {
			problem += ": " + facts.Error
		}
		fix = facts.Fix
		if fix == "" {
			fix = "Use a certificate from a public CA: with domain set and tls.mode acme Control obtains one itself."
		}
		return problem + ".", fix
	}
	return "", ""
}

func checkPanelCertificate(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckPanelCertificate, Level: LevelError}
	value, _ := effectiveControlURL(in)
	if in.PanelCertificate == nil {
		check.OK = true
		check.Message = "Control does not terminate https for " + value + " itself (tls.mode external): the CDN or reverse proxy in front of it serves the certificate nodes download the install script with."
		return check
	}
	if problem, fix := certificateProblem("panel", in.PanelCertificate); problem != "" {
		check.Message, check.Fix = problem, fix
		if in.PanelCertificate.State == CertificateValid {
			// Valid but not publicly trusted: a CDN's origin certificate
			// works behind the CDN, which nodes then go through; only nodes
			// that reach Control directly refuse it.
			check.Level = LevelWarning
			check.Message += " Behind a CDN that accepts it (an origin certificate) the command works; nodes that reach Control directly refuse it."
		}
		return check
	}
	check.OK = true
	check.Message = fmt.Sprintf("The panel certificate for %s is publicly trusted and valid until %s.", in.PanelCertificate.Name, in.PanelCertificate.NotAfter.UTC().Format(time.RFC3339))
	return check
}

func checkGRPCCertificate(in ReadinessInput) ReadinessCheck {
	check := ReadinessCheck{ID: CheckGRPCCertificate, Level: LevelError}
	facts := in.GRPCCertificate
	if in.GRPCEdge {
		check.OK = true
		check.Message = "The gRPC certificate is held by the edge proxy that grpc.edge_proxies names: Control cannot check it from here. Check its certificate and expiry on that proxy."
		return check
	}
	if facts == nil {
		check.Message = "The gRPC listener has no TLS certificate: nodes verify it against their system CAs and present their own certificate."
		check.Fix = "Set domain (Control obtains a publicly trusted certificate for it; with the panel behind a CDN also grpc.public_name, a DNS-only name for the nodes), or grpc.tls_cert_file and grpc.tls_key_file, then restart Control."
		return check
	}
	if problem, fix := certificateProblem("gRPC", facts); problem != "" {
		check.Message, check.Fix = problem, fix
		return check
	}
	if host, _, err := net.SplitHostPort(grpcTarget(in)); err == nil && host != "" && len(facts.Names) > 0 && !covers(facts.Names, host) {
		check.Message = fmt.Sprintf("Nodes dial %s, but the gRPC certificate covers %s: their TLS check fails.", host, strings.Join(facts.Names, ", "))
		check.Fix = "Make agent_install.grpc_target name a host the certificate covers (or leave it empty), or set grpc.public_name to that host."
		return check
	}
	check.OK = true
	check.Message = fmt.Sprintf("The gRPC certificate for %s is publicly trusted and valid until %s.", facts.Name, facts.NotAfter.UTC().Format(time.RFC3339))
	return check
}
