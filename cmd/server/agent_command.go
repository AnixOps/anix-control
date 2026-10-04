package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentinstall"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	grpcserver "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"gorm.io/gorm"
)

const agentCommandUsage = `usage:
  anix-control agent token create -node <proxy-<id>|forward-<id>> [-ttl 24h]
  anix-control agent link-ca list
  anix-control agent link-ca bundle
  anix-control agent link-ca rotate
  anix-control agent offline-bundle -arch <amd64|arm64> -o <file> [-control https://<control>]
  anix-control agent upgrade start [-version <tag>] [-exclude proxy-1,forward-2] [-exclude-tag <tag>,...] [-reason <text>] [-control https://<control>]
  anix-control agent upgrade status [-id <campaign>]
  anix-control agent upgrade pause|resume -id <campaign>
  anix-control agent upgrade abort -id <campaign> [-rollback]

token create prints a one-time agent enrollment credential (anixagt_...)
bound to the node, valid for at most 7 days. The agent presents it to
AgentEnrollment.Enroll on the gRPC listener. Agent enrollment needs the
built-in CA (module_runtime.ca_kek with pki: builtin); the module runtime
need not be enabled.

link-ca administers the forward link CA (H28), which signs the link
certificates forward engines present to each other: list prints its CAs,
bundle the PEM trust bundle nodes verify their peers with, and rotate
creates the next CA, which signs once it has been trusted for one link
certificate lifetime (7 days).

offline-bundle writes the offline install bundle of one architecture for
"install.sh --offline <file>": a tar.gz with this Control's
/install/agent.env, the Agent release zip with its .sig, SHA256SUMS and
SHA256SUMS.sig (from agent_install.artifact_dir/<tag>/, verified with
plugins.official_public_key), and the install script with its signature.
-control is the address nodes reach (default agent_install.public_url).

upgrade runs staged Agent upgrades (H19): start pushes the Agent release
(default this Control's, from agent_install.artifact_dir, verified with
plugins.official_public_key) to every enabled node whose Agent was seen on
the Agent Control stream, in batches of 5%, 25% and 100% of the nodes, at
least 30 minutes each, canaries first. A batch in which more than 5% of
the offered nodes fail, or do not reconnect with the new version within
10 minutes, is rolled back. The running Control's singleton worker drives
the campaign; status prints it (the active one, else the latest).

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

func agentUsageError() error {
	return fmt.Errorf("invalid agent command\n%s", agentCommandUsage)
}

// runAgentCommand administers the agent PKI from the command line. It prints
// JSON so scripts can pick out the credential.
func runAgentCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) > 1 && arguments[0] == "upgrade" {
		return runAgentUpgradeCommand(ctx, cfg, db, arguments[1], arguments[2:], stdout)
	}
	if len(arguments) > 0 && arguments[0] == "offline-bundle" {
		return runOfflineBundleCommand(cfg, arguments[1:], stdout)
	}
	if len(arguments) == 2 && arguments[0] == "link-ca" {
		return runLinkCACommand(ctx, cfg, db, arguments[1], stdout)
	}
	if len(arguments) < 2 || arguments[0]+" "+arguments[1] != "token create" {
		return agentUsageError()
	}
	flags := flag.NewFlagSet("agent token create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	nodeName := flags.String("node", "", "the node: proxy-<id> or forward-<id>")
	ttl := flags.Duration("ttl", agentpki.DefaultEnrollmentCredentialTTL, "credential lifetime (at most 168h)")
	if err := flags.Parse(arguments[2:]); err != nil {
		return fmt.Errorf("%w\n%s", err, agentCommandUsage)
	}
	if flags.NArg() != 0 {
		return agentUsageError()
	}
	node, err := agentcontrol.ParseAgentNode(*nodeName)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, agentCommandUsage)
	}
	pki, err := agentpki.FromConfig(cfg, db)
	if err != nil {
		return err
	}
	credential, row, err := pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: node, TTL: *ttl, Actor: "cli"})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]any{"enrollment": row, "credential": credential})
}

// runOfflineBundleCommand runs "agent offline-bundle": it writes the bundle
// to a temporary file next to -o and renames it into place, then prints
// what it wrote as JSON.
func runOfflineBundleCommand(cfg *config.Config, arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent offline-bundle", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	arch := flags.String("arch", "", "the node's architecture: amd64 or arm64")
	output := flags.String("o", "", "the bundle file to write (.tar.gz)")
	controlURL := flags.String("control", "", "the Control address nodes reach (default agent_install.public_url)")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("%w\n%s", err, agentCommandUsage)
	}
	if flags.NArg() != 0 || *arch == "" || *output == "" {
		return agentUsageError()
	}
	install := cfg.AgentInstall
	address := *controlURL
	if address == "" {
		address = install.PublicURL
	}
	settings, err := agentinstall.ResolveSettings(agentinstall.SettingsInput{
		ControlURL: address, GRPCTarget: install.GRPCTarget, GRPCPort: cfg.GRPC.Port,
		AgentVersion: install.AgentVersion, ReleaseVersion: handler.ReleaseVersion,
		ArtifactDir: install.ArtifactDir, CNMirrorURL: install.CNMirrorURL,
	})
	if err != nil {
		return fmt.Errorf("%w (or pass -control https://<control>)", err)
	}
	// The script's signature is optional: the bundle's own checks do not
	// depend on it; operators use it to verify install.sh.
	scriptSignature, _ := agentinstall.LoadSignature(install.SignatureFile, cfg.Plugins.OfficialPublicKey)
	target := filepath.Clean(*output)
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	bundle, err := agentinstall.WriteOfflineBundle(tmp, settings, *arch, cfg.Plugins.OfficialPublicKey, scriptSignature)
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		File string `json:"file"`
		agentinstall.OfflineBundle
		Install string `json:"install"`
	}{
		File: target, OfflineBundle: bundle,
		Install: "sudo bash install.sh --offline " + filepath.Base(target) + " --control " + settings.ControlURL + " --node <proxy-<id>|forward-<id>> --token <anixagt_...>",
	})
}

// runLinkCACommand runs "agent link-ca <command>".
func runLinkCACommand(ctx context.Context, cfg *config.Config, db *gorm.DB, command string, stdout io.Writer) error {
	link, err := agentpki.LinkAuthorityFromConfig(cfg, db)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	switch command {
	case "list":
		rows, err := link.CAs(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(rows)
	case "bundle":
		bundle, err := link.TrustBundle(ctx)
		if err != nil {
			return err
		}
		for _, certificate := range bundle {
			if err := pem.Encode(stdout, &pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}); err != nil {
				return err
			}
		}
		return nil
	case "rotate":
		next, err := link.Rotate(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(next)
	default:
		return agentUsageError()
	}
}

// agentPKIMaintenanceInterval is how often expired agent certificate
// records and unused expired enrollment credentials are pruned.
const agentPKIMaintenanceInterval = time.Hour

// startAgentPKIMaintenance prunes agent PKI records that can no longer
// authenticate: certificates and unused credentials expired for a day.
func (rt *serverRuntime) startAgentPKIMaintenance() {
	if rt.grpcSrv == nil || rt.grpcSrv.AgentPKI() == nil {
		return
	}
	pki := rt.grpcSrv.AgentPKI()
	rt.workers.Go("agent PKI maintenance", func(ctx context.Context) {
		for {
			if err := pki.Prune(ctx, time.Now().Add(-24*time.Hour)); err != nil && ctx.Err() == nil {
				log.Printf("Agent PKI prune failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(agentPKIMaintenanceInterval):
			}
		}
	})
}

// agentPKIForGRPC returns the agent PKI the gRPC listener verifies client
// certificates with: nil when the built-in CA is off (or the PKI is
// external), which every agent_control.mtls but an explicit required allows
// (the default required starts and warns, agentTransportPolicyLog). It needs
// the CA only, not the module runtime or its listener.
func agentPKIForGRPC(cfg *config.Config, db *gorm.DB) (*agentpki.Service, error) {
	pki, err := agentpki.FromConfig(cfg, db)
	switch {
	case errors.Is(err, agentpki.ErrDisabled):
		if mode := cfg.AgentControl.MTLSOrDefault(); mode == config.AgentMTLSRequired && cfg.AgentControl.MTLSExplicit() {
			return nil, fmt.Errorf("agent_control.mtls %q: %w", mode, err)
		}
		if errors.Is(err, agentpki.ErrExternalPKI) {
			log.Printf("Agent enrollment is off: %v", err)
		}
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("agent PKI: %w", err)
	}
	if cfg.GRPC.TLSCertFile == "" {
		log.Printf("Agent enrollment is served without TLS on the gRPC listener: agents can enroll, but cannot present client certificates until grpc.tls_cert_file is set")
	}
	return pki, nil
}

// agentTransportPolicyLog describes the effective agent_control policy at
// startup: the mode, what it means for legacy agents, the sunset and
// whether agents can enroll at all.
func agentTransportPolicyLog(cfg *config.Config, grpcSrv *grpcserver.Server) string {
	mode := cfg.AgentControl.MTLSOrDefault()
	var effect string
	switch mode {
	case config.AgentMTLSOff:
		effect = "client certificates are neither requested nor accepted; legacy API-key agents are served silently"
	case config.AgentMTLSOptional:
		effect = "client certificates are verified when presented; legacy API-key agents are served silently"
	case config.AgentMTLSPreferred:
		effect = "legacy API-key agents are served with deprecation signals (Deprecation/Sunset/Link headers, x-anix-auth-deprecated)"
	case config.AgentMTLSRequired:
		effect = "AnixOps Agent channels accept client certificates only; legacy agent paths answer 403 agent_mtls_required (UniProxy and v2board gRPC stay open)"
	}
	sunset := "none"
	if value := strings.TrimSpace(cfg.AgentControl.LegacySunset); value != "" {
		sunset = value
	}
	enrollment := "available"
	switch {
	case mode == config.AgentMTLSOff:
		enrollment = "off"
	case grpcSrv == nil:
		enrollment = "unavailable: the gRPC listener is off (grpc.enabled)"
	case grpcSrv.AgentPKI() == nil:
		enrollment = "unavailable: no built-in CA (module_runtime.ca_kek)"
	case strings.TrimSpace(cfg.GRPC.TLSCertFile) == "":
		enrollment = "unavailable for client certificates: no TLS on the gRPC listener (grpc.tls_cert_file)"
	}
	if mode == config.AgentMTLSRequired && enrollment != "available" {
		origin := "set"
		if !cfg.AgentControl.MTLSExplicit() {
			origin = "the default since v4.2"
		}
		return fmt.Sprintf("WARNING: Agent transports: agent_control.mtls=required (%s): %s; legacy sunset: %s; agent enrollment: %s. No AnixOps Agent can connect: give the kernel grpc.enabled, grpc.tls_cert_file/tls_key_file and module_runtime.ca_kek, or set agent_control.mtls: preferred while nodes migrate (docs/UPGRADE.md, \"Agent Transports: v4.2 Requires Enrolled Agents\").",
			origin, effect, sunset, enrollment)
	}
	next := "Check `anix-control agents transports --check-required` before setting required (the default from v4.2)."
	if mode == config.AgentMTLSRequired {
		next = "`anix-control agents transports --check-required` lists the nodes it refuses."
	}
	return fmt.Sprintf("Agent transports: agent_control.mtls=%s: %s; legacy sunset: %s; agent enrollment: %s. %s",
		mode, effect, sunset, enrollment, next)
}
