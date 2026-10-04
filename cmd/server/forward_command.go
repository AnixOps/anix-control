package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

const forwardCommandUsage = `usage:
  anix-control forward reset-node <node_ref>

reset-node moves a node's forwarding generation (node_ref proxy-<id> or
forward-<id>) above both the one Control stored and the one the node's
latest report says its Agent holds, keeping the node's hops, so the Agent
applies the node's current state again (after its rules were changed by
hand, or to drive a push again). A reset or restored Control database needs
no command: a node that reports a generation ahead of the stored one is
recovered on that report, within a minute (forward-sdk.md section 8.2). The
running Control sends the node its configuration at its next refresh,
within a minute. It prints the node's previous, reported and new generation
as JSON and writes the change to the audit log as system/cli.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

func forwardUsageError() error {
	return fmt.Errorf("invalid forward command\n%s", forwardCommandUsage)
}

// forwardResetOutput is what forward reset-node prints.
type forwardResetOutput struct {
	kernelforward.NodeReset
	Audited bool `json:"audited"`
}

// runForwardCommand administers the kernel's forwarding state.
func runForwardCommand(ctx context.Context, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) != 2 || arguments[0] != "reset-node" {
		return forwardUsageError()
	}
	reset, err := kernelforward.New(db).ResetNode(ctx, arguments[1])
	if err != nil {
		return err
	}
	content, _ := json.Marshal(reset)
	auditErr := service.NewOperationLogService(db.WithContext(ctx)).Record(&service.OperationLogInput{
		Username: service.RouteModeCLIActor, Action: "forward.reset_node", Module: "forward",
		TargetType: "forward_node_state", Content: string(content),
	})
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(forwardResetOutput{NodeReset: reset, Audited: auditErr == nil})
}
