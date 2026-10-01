package kernelnodeops

import (
	"context"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// Retirements executes RetireNode and RetireProtocol (node-ops-service.md
// section 3.3, NO-5; decision D11). The kernel performs the cascade of the
// legacy deletion in one transaction: a node's protocols with their users'
// WireGuard peers, their subscription group links and their secrets, the
// node's credentials and raw configuration secrets, and its agent
// certificates and enrollments; a protocol's peers, links and secrets. The
// package deletes its own row afterwards. The functions are the legacy
// deletions' (service.RetireProxyNodeTx, RetireForwardNodeTx,
// RetireProtocolTx), and every step is idempotent: a retirement of what is
// already gone succeeds with nothing counted.
type Retirements struct{}

// Register serves the retirement kinds on registry.
func (r *Retirements) Register(registry *Registry) error {
	for _, served := range []struct {
		kind     string
		executor ExecutorFunc
	}{
		{KindNodeRetire, r.retireNode},
		{KindProtocolRetire, r.retireProtocol},
	} {
		if err := registry.Register(served.kind, served.executor); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	if err := (&Retirements{}).Register(DefaultExecutors); err != nil {
		panic(err)
	}
}

func retireResult(counts service.RetireCounts) *kernelnodeopsv1.OperationResult {
	return &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Retire{Retire: &kernelnodeopsv1.RetireResult{
		CredentialsRevoked: counter32(counts.CredentialsRevoked), SecretsDeleted: counter32(counts.SecretsDeleted),
		WireguardPeersDeleted: counter32(counts.WireGuardPeersDeleted), ProtocolsDeleted: counter32(counts.ProtocolsDeleted),
		SubscriptionLinksDeleted: counter32(counts.SubscriptionLinksDeleted),
	}}}
}

// counter32 answers a row count; a negative one is 0.
func counter32(value int64) uint32 {
	if value < 0 || value > int64(^uint32(0)) {
		return 0
	}
	return uint32(value)
}

// retireNode executes RetireNode for a proxy or a forward node.
func (r *Retirements) retireNode(ctx context.Context, run *Run) Outcome {
	node := run.Operation.GetRetireNode().GetNode()
	id := nodeID(node.GetId())
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	var counts service.RetireCounts
	outcome, ok := transact(ctx, run, "retiring the node", func(tx *gorm.DB) (err error) {
		if node.GetKind() == kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD {
			counts, err = service.RetireForwardNodeTx(tx, id)
		} else {
			counts, err = service.RetireProxyNodeTx(tx, id)
		}
		return err
	})
	if !ok {
		return outcome
	}
	return Succeeded(retireResult(counts))
}

// retireProtocol executes RetireProtocol.
func (r *Retirements) retireProtocol(ctx context.Context, run *Run) Outcome {
	id := nodeID(run.Operation.GetRetireProtocol().GetProtocolId())
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	var counts service.RetireCounts
	outcome, ok := transact(ctx, run, "retiring the protocol", func(tx *gorm.DB) (err error) {
		counts, err = service.RetireProtocolTx(tx, id)
		return err
	})
	if !ok {
		return outcome
	}
	return Succeeded(retireResult(counts))
}
