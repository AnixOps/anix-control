package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"gorm.io/gorm"
)

const agentCommandUsage = `usage:
  anix-control agent token create -node <proxy-<id>|forward-<id>> [-ttl 24h]

token create prints a one-time agent enrollment credential (anixagt_...)
bound to the node, valid for at most 7 days. The agent presents it to
AgentEnrollment.Enroll on the gRPC listener. Agent enrollment needs the
built-in module PKI (module_runtime.enabled with pki: builtin).

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

func agentUsageError() error {
	return fmt.Errorf("invalid agent command\n%s", agentCommandUsage)
}

// runAgentCommand administers the agent PKI from the command line. It prints
// JSON so scripts can pick out the credential.
func runAgentCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
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
// certificates with: nil, with a notice, when the built-in module PKI is
// off, which agent_control.mtls optional allows.
func agentPKIForGRPC(cfg *config.Config, db *gorm.DB) (*agentpki.Service, error) {
	pki, err := agentpki.FromConfig(cfg, db)
	switch {
	case errors.Is(err, agentpki.ErrDisabled):
		if mode := cfg.AgentControl.MTLSOrDefault(); mode != config.AgentMTLSOptional {
			return nil, fmt.Errorf("agent_control.mtls %q: %w", mode, err)
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
