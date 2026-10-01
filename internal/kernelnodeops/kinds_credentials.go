package kernelnodeops

import (
	"context"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// The names a credential operation reveals a generated secret under
// (SecretHandle.field): the answer fields config/node-secret-fields.json
// lists for the routes that show them.
const (
	RevealAPIKey     = "api_key"
	RevealSecret     = "secret"
	RevealAPIToken   = "api_token"
	RevealKey        = "key"
	RevealToken      = "token"
	RevealCredential = "credential"
)

// The request fields a typed credential is sealed from, as the field list
// spells them.
const (
	fieldForwardToken = "/api_token"
	fieldAPIKey       = "/api_key"
	fieldSecret       = "/secret"
)

// AgentEnrollmentActor names the kernel's KernelNodeOps in the audit
// entry of an enrollment credential a package had issued.
const AgentEnrollmentActor = "kernelnodeops"

// Credentials executes the credentials family (node-ops-service.md section
// 3.3, NO-5): IssueCredential, RevokeCredential, IssueRegistrationKey,
// RevokeRegistrationKey and IssueCleanAgent. Each runs the function of
// internal/service the legacy route runs, in one transaction with every
// nodesecrets.Sync and every agent certificate revocation, so native and
// legacy write identical rows.
//
// Secrets in, secrets out. A typed secret arrives as a sealed handle: the
// Preparer resolves it for the bound request (Submission.Unseal) and keeps
// it for the executor in memory. A generated secret is minted as a handle
// (Run.Reveal) inside the transaction that stores it: when the answer
// cannot show it, nothing is stored. A credential is issued only in an
// administrator's request (the binding), never outside one, and results
// never carry a value in clear.
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
		{KindRegKeyIssue, prepared{prepare: requireBinding, execute: c.issueRegistrationKey}},
		{KindRegKeyRevoke, ExecutorFunc(c.revokeRegistrationKey)},
		{KindCleanAgentIssue, prepared{prepare: requireBinding, execute: c.issueCleanAgent}},
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

func requireBinding(_ context.Context, submission *Submission) error {
	if submission.Request == nil {
		return errNoBinding
	}
	return nil
}

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

// typedCredential is what prepareIssue keeps for the executor.
type typedCredential struct {
	value string
}

// credentialField is the request field a credential of kind is sealed
// from, for a subject of the given node kind.
func credentialField(subject kernelnodeopsv1.NodeKind, kind kernelnodeopsv1.CredentialKind) string {
	switch {
	case subject == kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD:
		return fieldForwardToken
	case kind == kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_SHARED_SECRET:
		return fieldSecret
	default:
		return fieldAPIKey
	}
}

// prepareIssue needs the administrator's request, resolves a typed
// credential's handle for the subject, and refuses to replace a credential
// the operation did not ask to replace (FAILED_PRECONDITION).
func (c *Credentials) prepareIssue(_ context.Context, submission *Submission) error {
	if submission.Request == nil {
		return errNoBinding
	}
	op := submission.Operation.GetIssueCredential()
	if op.GetKind() != kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT && !op.GetReplace() {
		exists, err := c.credentialExists(submission, op.GetSubject(), op.GetKind())
		if err != nil {
			return err
		}
		if exists {
			return status.Error(codes.FailedPrecondition, "the node already holds this credential: set replace to rotate it")
		}
	}
	if op.GetValue() == nil {
		return nil
	}
	secrets, err := submission.Unseal(sealedsecrets.Use{
		Handle: op.GetValue().GetHandle(), Target: NodeTarget(op.GetSubject()),
		Field: credentialField(op.GetSubject().GetKind(), op.GetKind()),
	})
	if err != nil {
		return err
	}
	if secrets[0].JSON || secrets[0].Value == "" {
		return status.Error(codes.InvalidArgument, "a credential is one non-empty string")
	}
	submission.Retain(typedCredential{value: secrets[0].Value})
	op.Value.Handle = SealedPrefix
	return nil
}

// credentialExists reports whether the subject holds the credential, in
// the submitting call.
func (c *Credentials) credentialExists(submission *Submission, subject *kernelnodeopsv1.NodeRef, kind kernelnodeopsv1.CredentialKind) (bool, error) {
	db := submission.db
	if db == nil {
		return false, nil
	}
	id := nodeID(subject.GetId())
	switch subject.GetKind() {
	case kernelnodeopsv1.NodeKind_NODE_KIND_PROXY:
		apiKey, secret, err := service.ProxyNodeHasCredentials(db, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		switch kind {
		case kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY:
			return apiKey, nil
		case kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_SHARED_SECRET:
			return secret, nil
		}
		return apiKey || secret, nil
	case kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD:
		has, err := service.ForwardNodeHasToken(db, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return has, err
	}
	return false, nil
}

// currentCredential answers the new-table row of a subject's credential:
// its id and version for the result.
func currentCredential(tx *gorm.DB, subjectKind string, subjectID uint, kind string) (uint64, uint32) {
	var rows []model.NodeCredential
	if err := tx.Where("subject_kind = ? AND subject_id = ? AND kind = ? AND status <> ?", subjectKind, uint64(subjectID), kind, nodesecrets.StatusRetired).
		Order("version DESC").Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return 0, 0
	}
	return rows[0].ID, uint32(max(rows[0].Version, 0)) // #nosec G115 -- versions count from 1.
}

// issue executes IssueCredential.
func (c *Credentials) issue(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetIssueCredential()
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	subject := op.GetSubject()
	id := nodeID(subject.GetId())
	result := &kernelnodeopsv1.CredentialResult{Subject: nodeRef(subject.GetKind(), subject.GetId()), Kind: op.GetKind()}
	var typed *typedCredential
	if op.GetValue() != nil {
		kept, ok := run.Prepared().(typedCredential)
		if !ok {
			return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_SECRET_HANDLE_INVALID,
				"the typed credential is no longer held: its request ended or the kernel restarted since the submission; nothing was issued", false)
		}
		typed = &kept
		run.UseSecret(kept.value)
	}
	if op.GetKind() == kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT {
		return c.issueEnrollment(ctx, run, result)
	}
	switch subject.GetKind() {
	case kernelnodeopsv1.NodeKind_NODE_KIND_PROXY:
		return c.issueProxy(ctx, run, id, typed, result)
	case kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD:
		return c.issueForward(ctx, run, id, typed, result)
	}
	return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "a clean agent's token is issued with IssueCleanAgent", false)
}

func (c *Credentials) issueProxy(ctx context.Context, run *Run, id uint, typed *typedCredential, result *kernelnodeopsv1.CredentialResult) Outcome {
	kind := result.GetKind()
	wantKey := kind != kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_SHARED_SECRET
	wantSecret := kind != kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY
	outcome, ok := transact(ctx, run, "issuing the node's credentials", func(tx *gorm.DB) error {
		hasKey, hasSecret, err := service.ProxyNodeHasCredentials(tx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, fmt.Sprintf("proxy node %d not found", id), false)
		}
		if err != nil {
			return err
		}
		replacing := (wantKey && hasKey) || (wantSecret && hasSecret)
		if replacing && !run.Operation.GetIssueCredential().GetReplace() {
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "the node already holds this credential: set replace to rotate it", false)
		}
		var credentials service.ProxyNodeCredentials
		var reveals []*kernelnodeopsv1.SecretHandle
		if typed != nil {
			if wantKey {
				credentials.APIKey = typed.value
			} else {
				credentials.Secret = typed.value
			}
		} else {
			if credentials, err = service.GenerateProxyNodeCredentials(wantKey, wantSecret); err != nil {
				return err
			}
			run.UseSecret(credentials.APIKey, credentials.Secret)
			for _, generated := range []struct{ name, value string }{{RevealAPIKey, credentials.APIKey}, {RevealSecret, credentials.Secret}} {
				if generated.value == "" {
					continue
				}
				handle, err := revealHandle(run, generated.name, generated.value)
				if err != nil {
					return err
				}
				reveals = append(reveals, handle)
			}
		}
		if err := service.IssueProxyNodeCredentialsTx(tx, id, credentials, replacing); err != nil {
			return err
		}
		resultKind := nodesecrets.KindNodeAPIKey
		if !wantKey {
			resultKind = nodesecrets.KindNodeSharedSecret
		}
		result.CredentialId, result.Version = currentCredential(tx, nodesecrets.SubjectProxy, id, resultKind)
		result.Reveal = reveals
		return nil
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: result}})
}

func (c *Credentials) issueForward(ctx context.Context, run *Run, id uint, typed *typedCredential, result *kernelnodeopsv1.CredentialResult) Outcome {
	outcome, ok := transact(ctx, run, "issuing the forward node's token", func(tx *gorm.DB) error {
		has, err := service.ForwardNodeHasToken(tx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, fmt.Sprintf("forward node %d not found", id), false)
		}
		if err != nil {
			return err
		}
		if has && !run.Operation.GetIssueCredential().GetReplace() {
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "the node already holds a token: set replace to rotate it", false)
		}
		var token string
		var reveals []*kernelnodeopsv1.SecretHandle
		if typed != nil {
			token = typed.value
		} else {
			if token, err = service.GenerateForwardNodeToken(); err != nil {
				return err
			}
			run.UseSecret(token)
			handle, err := revealHandle(run, RevealAPIToken, token)
			if err != nil {
				return err
			}
			reveals = append(reveals, handle)
		}
		if _, err := service.IssueForwardNodeTokenTx(tx, id, token); err != nil {
			return err
		}
		result.CredentialId, result.Version = currentCredential(tx, nodesecrets.SubjectForward, id, nodesecrets.KindForwardNodeToken)
		result.Reveal = reveals
		return nil
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: result}})
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

// revoke executes RevokeCredential: a clean agent's token (the legacy
// revocation), or a node's agent enrollments and certificates. A proxy
// node's key and secret and a forward node's token are never revoked in
// place: no kernel route does, and a node without them cannot be told from
// one never issued. They are rotated (IssueCredential with replace) or
// retired with the node.
func (c *Credentials) revoke(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetRevokeCredential()
	subject := op.GetSubject()
	id := nodeID(subject.GetId())
	result := &kernelnodeopsv1.CredentialResult{Subject: nodeRef(subject.GetKind(), subject.GetId()), Kind: op.GetKind()}
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	outcome, ok := transact(ctx, run, "revoking the credential", func(tx *gorm.DB) error {
		switch {
		case subject.GetKind() == kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT:
			err := service.RevokeCleanAgentTx(tx, id)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, fmt.Sprintf("clean agent %d not found", id), false)
			}
			if err != nil {
				return err
			}
			result.CredentialId, result.Version = currentCredential(tx, nodesecrets.SubjectCleanAgent, id, nodesecrets.KindCleanAgentToken)
			return nil
		case op.GetKind() == kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT:
			return agentpki.RevokeNode(ctx, tx, agentNode(subject), agentpki.RevokeReasonCredentialsRevoked)
		}
		return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED,
			"a node's key, secret or token is not revoked in place: rotate it with IssueCredential (replace) or retire the node; its agent enrollments are revoked with kind AGENT_ENROLLMENT", false)
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: result}})
}

// issueRegistrationKey executes IssueRegistrationKey, as the registration
// key routes do, and reveals the key.
func (c *Credentials) issueRegistrationKey(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetIssueRegistrationKey()
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	result := &kernelnodeopsv1.RegistrationKeyResult{Name: op.GetName(), ExpiresAtUnix: op.GetExpiresAtUnix()}
	outcome, ok := transact(ctx, run, "issuing the registration key", func(tx *gorm.DB) error {
		var expireAt *int64
		if op.GetExpiresAtUnix() > 0 {
			value := op.GetExpiresAtUnix()
			expireAt = &value
		}
		row, key, err := service.IssueRegistrationKeyTx(tx, op.GetName(), expireAt)
		if err != nil {
			return err
		}
		run.UseSecret(key)
		handle, err := revealHandle(run, RevealKey, key)
		if err != nil {
			return err
		}
		result.KeyId = uint64(row.ID)
		result.Reveal = []*kernelnodeopsv1.SecretHandle{handle}
		return nil
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_RegistrationKey{RegistrationKey: result}})
}

// revokeRegistrationKey executes RevokeRegistrationKey, as the key deletion
// route does. A key gone since the submission is nothing to do.
func (c *Credentials) revokeRegistrationKey(ctx context.Context, run *Run) Outcome {
	id := nodeID(run.Operation.GetRevokeRegistrationKey().GetKeyId())
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	result := &kernelnodeopsv1.RegistrationKeyResult{KeyId: uint64(id)}
	outcome, ok := transact(ctx, run, "revoking the registration key", func(tx *gorm.DB) error {
		var rows []model.AuthorizedKey
		if err := tx.Select("id", "name", "expire_at").Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			result.Name = rows[0].Name
			if rows[0].ExpireAt != nil {
				result.ExpiresAtUnix = *rows[0].ExpireAt
			}
		}
		_, err := service.RevokeRegistrationKeyTx(tx, id)
		return err
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_RegistrationKey{RegistrationKey: result}})
}

// issueCleanAgent executes IssueCleanAgent, as the clean agent creation
// does, and reveals the agent's token. The result's subject names the new
// agent.
func (c *Credentials) issueCleanAgent(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetIssueCleanAgent()
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	result := &kernelnodeopsv1.CredentialResult{Kind: kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_CLEAN_AGENT_TOKEN}
	outcome, ok := transact(ctx, run, "issuing the clean agent", func(tx *gorm.DB) error {
		forwardNodeID := nodeID(op.GetForwardNodeId())
		created, err := service.CreateCleanAgentTokenTx(tx, service.ForwardCleanAgentCreateInput{Name: op.GetName(), NodeID: &forwardNodeID})
		switch {
		case errors.Is(err, service.ErrForwardCleanAgentNodeNotFound):
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, fmt.Sprintf("forward node %d not found", forwardNodeID), false)
		case errors.Is(err, service.ErrForwardCleanAgentNodeRequired):
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
		case err != nil:
			return err
		}
		run.UseSecret(created.Token)
		handle, err := revealHandle(run, RevealToken, created.Token)
		if err != nil {
			return err
		}
		result.Subject = nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, uint64(created.Agent.ID))
		result.CredentialId, result.Version = currentCredential(tx, nodesecrets.SubjectCleanAgent, created.Agent.ID, nodesecrets.KindCleanAgentToken)
		result.Reveal = []*kernelnodeopsv1.SecretHandle{handle}
		return nil
	})
	if !ok {
		return outcome
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: result}})
}
