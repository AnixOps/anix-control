package kernelnodeops

import (
	"context"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// RevealCredential is the name an issued enrollment credential is revealed
// under (SecretHandle.field).
const RevealCredential = "credential"

// AgentEnrollmentActor names the kernel's KernelNodeOps in the audit
// entry of an enrollment credential a package had issued.
const AgentEnrollmentActor = "kernelnodeops"

// Credentials executes the credentials family (node-ops-service.md section
// 3.3, NO-5): IssueCredential and RevokeCredential of a node's agent
// enrollment, the only credential a node has. Each runs in one transaction
// with its agent certificate revocations.
//
// An issued credential is minted as a handle (Run.Reveal) inside the
// transaction that stores it: when the answer cannot show it, nothing is
// stored. A credential is issued only in an administrator's request (the
// binding), never outside one, and results never carry a value in clear.
type Credentials struct {
	// PKI returns the agent PKI that issues enrollment credentials, on the
	// operation's transaction; nil uses the kernel's configuration
	// (agentpki.FromConfig).
	PKI func(ctx context.Context, tx *gorm.DB) (*agentpki.Service, error)
}

// Register serves the credential kinds on registry.
func (c *Credentials) Register(registry *Registry) error {
	for _, served := range []struct {
		kind     string
		executor Executor
	}{
		{KindCredentialIssue, prepared{prepare: c.prepareIssue, execute: c.issue}},
		{KindCredentialRevoke, ExecutorFunc(c.revoke)},
	} {
		if err := registry.Register(served.kind, served.executor); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	if err := (&Credentials{}).Register(DefaultExecutors); err != nil {
		panic(err)
	}
}

// prepared is an executor with a Preparer.
type prepared struct {
	prepare func(context.Context, *Submission) error
	execute func(context.Context, *Run) Outcome
}

func (p prepared) Prepare(ctx context.Context, submission *Submission) error {
	return p.prepare(ctx, submission)
}

func (p prepared) Execute(ctx context.Context, run *Run) Outcome { return p.execute(ctx, run) }

// errNoBinding refuses an issue outside an administrator's request: the
// secret it generates would be shown to nobody.
var errNoBinding = status.Error(codes.PermissionDenied, "issuing a credential needs the request binding of the administrator's request its value is answered to")

// outcomeError carries an outcome out of a transaction, which it rolls
// back.
type outcomeError struct{ outcome Outcome }

func (e outcomeError) Error() string { return e.outcome.err.GetMessage() }

func fail(code kernelnodeopsv1.ErrorCode, message string, retryable bool) error {
	return outcomeError{outcome: Failed(code, message, retryable)}
}

// transact runs body in a transaction of the engine's database and maps
// its error: an outcomeError is the outcome it carries, a cancelled context
// is Cancelled, anything else is INTERNAL and retryable with what, never a
// value.
func transact(ctx context.Context, run *Run, what string, body func(tx *gorm.DB) error) (Outcome, bool) {
	err := run.engine.DB.WithContext(ctx).Transaction(body)
	if err == nil {
		return Outcome{}, true
	}
	var failed outcomeError
	if errors.As(err, &failed) {
		return failed.outcome, false
	}
	if ctx.Err() != nil {
		return Cancelled(what + " stopped before it completed; nothing was written"), false
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, what+" failed: "+err.Error(), true), false
}

// revealHandle mints a generated secret's handle; failing that, the transaction
// rolls back: a credential nobody is shown is not issued.
func revealHandle(run *Run, name, value string) (*kernelnodeopsv1.SecretHandle, error) {
	handle, err := run.Reveal(name, value)
	if err != nil {
		return nil, fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_SECRET_HANDLE_INVALID,
			fmt.Sprintf("the generated %s cannot be shown: the request binding ended, or the bound route's answer lists no %q field; nothing was issued", name, name), false)
	}
	return handle, nil
}

// prepareIssue needs the administrator's request.
func (c *Credentials) prepareIssue(_ context.Context, submission *Submission) error {
	if submission.Request == nil {
		return errNoBinding
	}
	return nil
}

// issue executes IssueCredential.
func (c *Credentials) issue(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetIssueCredential()
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	subject := op.GetSubject()
	result := &kernelnodeopsv1.CredentialResult{Subject: nodeRef(subject.GetKind(), subject.GetId()), Kind: op.GetKind()}
	return c.issueEnrollment(ctx, run, result)
}

// agentNode is the agent PKI's name of a node.
func agentNode(ref *kernelnodeopsv1.NodeRef) agentcontrol.AgentNode {
	kind := agentcontrol.NodeKindProxy
	if ref.GetKind() == kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD {
		kind = agentcontrol.NodeKindForward
	}
	return agentcontrol.AgentNode{Kind: kind, ID: uint32(nodeID(ref.GetId()))} // #nosec G115 -- the kinds' checks bound ids to 32 bits.
}

// pki returns the agent PKI on tx.
func (c *Credentials) pki(ctx context.Context, tx *gorm.DB) (*agentpki.Service, error) {
	if c.PKI != nil {
		return c.PKI(ctx, tx)
	}
	return agentpki.FromConfig(config.Get(), tx)
}

// issueEnrollment mints a one-time agent enrollment credential bound to
// the subject (section 5.3), through internal/agentpki, with its audit
// entry, and reveals it as a handle.
func (c *Credentials) issueEnrollment(ctx context.Context, run *Run, result *kernelnodeopsv1.CredentialResult) Outcome {
	outcome, ok := transact(ctx, run, "issuing the enrollment credential", func(tx *gorm.DB) error {
		pki, err := c.pki(ctx, tx)
		if errors.Is(err, agentpki.ErrDisabled) {
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
		}
		if err != nil {
			return err
		}
		request := agentpki.TokenRequest{Node: agentNode(result.GetSubject()), Actor: AgentEnrollmentActor + ":" + run.PackageID}
		if run.Request != nil && run.Request.Admin && run.Request.ActorID <= uint64(^uint32(0)) {
			request.CreatedBy = uint(run.Request.ActorID)
		}
		credential, _, err := pki.CreateEnrollmentToken(ctx, request)
		if errors.Is(err, agentpki.ErrInvalidNode) {
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, err.Error(), false)
		}
		if err != nil {
			return err
		}
		run.UseSecret(credential)
		handle, err := revealHandle(run, RevealCredential, credential)
		if err != nil {
			return err
		}
		result.Reveal = []*kernelnodeopsv1.SecretHandle{handle}
		return nil
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: result}})
}

// revoke executes RevokeCredential: a node's agent enrollments and
// certificates.
func (c *Credentials) revoke(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetRevokeCredential()
	subject := op.GetSubject()
	result := &kernelnodeopsv1.CredentialResult{Subject: nodeRef(subject.GetKind(), subject.GetId()), Kind: op.GetKind()}
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	outcome, ok := transact(ctx, run, "revoking the credential", func(tx *gorm.DB) error {
		return agentpki.RevokeNode(ctx, tx, agentNode(subject), agentpki.RevokeReasonCredentialsRevoked)
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: result}})
}
