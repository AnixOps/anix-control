package kernelnodeops

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Sealed secrets (node-ops-service.md section 3.7, NO-4). The gateway seals
// the node secrets of an administrator's request into handles bound to it;
// a submission carrying that request's binding resolves them in its
// Preparer (Submission.Unseal), and an executor mints the handles of the
// secrets its answer shows once (Run.Reveal). Handles never reach the
// ledger: a stored result keeps a SecretHandle's field and expiry, not its
// handle, which is answered to the submitting call only.

// ErrNoRequestBinding is Run.Reveal without a live request binding: the
// submission carried none, its request ended, or the kernel restarted.
var ErrNoRequestBinding = errors.New("the operation has no live request binding")

// BoundRequest is a verified request binding: the live v2 request the
// calling package is serving, as the kernel's gateway dispatched it. It is
// never stored.
type BoundRequest struct {
	RequestID string
	RouteID   string
	// ActorID and Admin are the request's kernel-authenticated principal.
	ActorID  uint64
	Admin    bool
	Deadline time.Time
	binding  sealedsecrets.Binding
}

// verifyBinding verifies a request binding on the generation the call
// arrived on: it must name a live dispatch to the calling host (not
// consumed, revoked or expired). None is nil; any other is
// PERMISSION_DENIED.
func (h *hostServer) verifyBinding(ctx context.Context, binding *kernelnodeopsv1.RequestBinding) (*BoundRequest, error) {
	raw := binding.GetBridgeCapability()
	if len(raw) == 0 {
		return nil, nil
	}
	refused := status.Error(codes.PermissionDenied, "the request binding names no live request of this package")
	identity, request, err := packagebridge.BoundRequest(ctx, raw)
	if err != nil || identity != h.host {
		return nil, refused
	}
	var principal struct {
		ActorID   uint64 `json:"actor_id"`
		Admin     bool   `json:"admin"`
		PackageID string `json:"package_id"`
	}
	if err := json.Unmarshal(request.PrincipalJSON, &principal); err != nil || principal.PackageID != h.host.PackageID {
		return nil, refused
	}
	return &BoundRequest{
		RequestID: request.RequestID, RouteID: request.RouteID, ActorID: principal.ActorID, Admin: principal.Admin,
		Deadline: request.Deadline,
		binding: sealedsecrets.Binding{
			Key: request.SealedRequest, PackageID: h.host.PackageID, Generation: h.host.Generation,
			RequestID: request.RequestID, RouteID: request.RouteID,
		},
	}, nil
}

// secretStore is the store handles are resolved from and minted into.
func (e *Engine) secretStore() *sealedsecrets.Store {
	if e.Secrets == nil {
		return sealedsecrets.DefaultStore()
	}
	return e.Secrets
}

// NodeTarget is the sealed secret target of a node reference.
func NodeTarget(ref *kernelnodeopsv1.NodeRef) sealedsecrets.Target {
	switch ref.GetKind() {
	case kernelnodeopsv1.NodeKind_NODE_KIND_PROXY:
		return sealedsecrets.Target{Kind: sealedsecrets.TargetProxy, ID: ref.GetId()}
	case kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD:
		return sealedsecrets.Target{Kind: sealedsecrets.TargetForward, ID: ref.GetId()}
	case kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT:
		return sealedsecrets.Target{Kind: sealedsecrets.TargetCleanAgent, ID: ref.GetId()}
	}
	return sealedsecrets.Target{}
}

// Unseal resolves sealed handles the bound request carried, for the target
// and field each is stored for, all or none, and each once
// (sealedsecrets.Store.Resolve). Without a binding, or for a handle sealed
// from another request, route, target or field, it is PERMISSION_DENIED;
// the error names no handle.
func (s *Submission) Unseal(uses ...sealedsecrets.Use) ([]sealedsecrets.Secret, error) {
	if s == nil || s.Request == nil {
		return nil, status.Error(codes.PermissionDenied, "sealed handles resolve only with the request binding they were sealed for")
	}
	secrets, err := s.store.Resolve(s.Request.binding, uses...)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "a sealed handle was refused: it is not one this request sealed for this target and field, or it was used")
	}
	return secrets, nil
}

// Reveal mints the handle of a secret the operation generated, for the
// answer to its bound request to show once under name (a SecretHandle
// field the route's answer lists). The value is also scrubbed from the
// operation's texts. The handle is answered to the submitting call only.
func (r *Run) Reveal(name, value string) (*kernelnodeopsv1.SecretHandle, error) {
	r.UseSecret(value)
	if r.Request == nil || !r.engine.now().Before(r.Request.Deadline) {
		return nil, ErrNoRequestBinding
	}
	handle, expires, err := r.engine.secretStore().Mint(r.Request.binding, name, value)
	if err != nil {
		return nil, err
	}
	return &kernelnodeopsv1.SecretHandle{Field: name, Handle: handle, ExpiresAtUnixMs: expires.UnixMilli()}, nil
}

// sealedState is what the engine holds in memory for sealed secrets: the
// bindings of operations whose request is live, and the handles results
// reveal to their submitting call.
type sealedState struct {
	mu       sync.Mutex
	bindings map[string]*BoundRequest
	reveals  map[string]reveal
}

type reveal struct {
	result *kernelnodeopsv1.OperationResult
	key    string
	until  time.Time
}

func bindingKey(packageID, requestID string) string { return packageID + "\x00" + requestID }

// retainBinding keeps an operation's binding until its request ends, for
// its executor; it reports whether it was kept.
func (e *Engine) retainBinding(packageID, requestID string, bound *BoundRequest) bool {
	if bound == nil {
		return false
	}
	e.sealed.mu.Lock()
	defer e.sealed.mu.Unlock()
	if e.sealed.bindings == nil {
		e.sealed.bindings = map[string]*BoundRequest{}
	}
	e.purgeSealedLocked()
	key := bindingKey(packageID, requestID)
	if _, exists := e.sealed.bindings[key]; exists {
		return false
	}
	e.sealed.bindings[key] = bound
	return true
}

func (e *Engine) dropBinding(packageID, requestID string) {
	e.sealed.mu.Lock()
	defer e.sealed.mu.Unlock()
	delete(e.sealed.bindings, bindingKey(packageID, requestID))
}

// boundRequest is an operation's binding while its request is live.
func (e *Engine) boundRequest(packageID, requestID string) *BoundRequest {
	e.sealed.mu.Lock()
	defer e.sealed.mu.Unlock()
	bound := e.sealed.bindings[bindingKey(packageID, requestID)]
	if bound == nil || !e.now().Before(bound.Deadline) {
		return nil
	}
	return bound
}

func (e *Engine) purgeSealedLocked() {
	now := e.now()
	for key, bound := range e.sealed.bindings {
		if !now.Before(bound.Deadline) {
			delete(e.sealed.bindings, key)
		}
	}
	for id, kept := range e.sealed.reveals {
		if !now.Before(kept.until) {
			delete(e.sealed.reveals, id)
		}
	}
}

// keepReveal keeps a result with its handles for the submitting call.
func (e *Engine) keepReveal(operationID string, result *kernelnodeopsv1.OperationResult, bound *BoundRequest) {
	e.sealed.mu.Lock()
	defer e.sealed.mu.Unlock()
	if e.sealed.reveals == nil {
		e.sealed.reveals = map[string]reveal{}
	}
	e.purgeSealedLocked()
	e.sealed.reveals[operationID] = reveal{result: result, key: bound.binding.Key, until: bound.Deadline}
}

func (e *Engine) hasReveal(operationID string) bool {
	e.sealed.mu.Lock()
	defer e.sealed.mu.Unlock()
	_, ok := e.sealed.reveals[operationID]
	return ok
}

// takeReveal returns, once, the revealed result of an operation to a call
// bound to the request it was minted for.
func (e *Engine) takeReveal(operationID string, bound *BoundRequest) *kernelnodeopsv1.OperationResult {
	if bound == nil {
		return nil
	}
	e.sealed.mu.Lock()
	defer e.sealed.mu.Unlock()
	kept, ok := e.sealed.reveals[operationID]
	if !ok || kept.key == "" || kept.key != bound.binding.Key || !e.now().Before(kept.until) {
		return nil
	}
	delete(e.sealed.reveals, operationID)
	return kept.result
}

// withoutHandles splits a result: the copy the ledger stores, whose
// SecretHandle messages keep their field and expiry but no handle, and,
// when it held any handle, the result as the executor made it.
func withoutHandles(result *kernelnodeopsv1.OperationResult) (stored, revealed *kernelnodeopsv1.OperationResult) {
	if result == nil {
		return nil, nil
	}
	copied := proto.Clone(result).(*kernelnodeopsv1.OperationResult)
	if !clearHandles(copied.ProtoReflect()) {
		return result, nil
	}
	return copied, result
}

var secretHandleName = (&kernelnodeopsv1.SecretHandle{}).ProtoReflect().Descriptor().FullName()

// clearHandles empties the handle of every SecretHandle in message and
// reports whether there was one.
func clearHandles(message protoreflect.Message) bool {
	if message.Descriptor().FullName() == secretHandleName {
		handle := message.Descriptor().Fields().ByName("handle")
		if message.Get(handle).String() == "" {
			return false
		}
		message.Clear(handle)
		return true
	}
	cleared := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsMap():
		case field.IsList() && field.Message() != nil:
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if clearHandles(list.Get(i).Message()) {
					cleared = true
				}
			}
		case field.Message() != nil:
			if clearHandles(value.Message()) {
				cleared = true
			}
		}
		return true
	})
	return cleared
}

// holdsHandle reports whether a message holds a sealed handle in any string
// or bytes field: what the ledger stores must not. The canonical form keeps
// only the bare SealedPrefix where a handle was (canonical), which is none.
func holdsHandle(message protoreflect.Message) bool {
	found := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsMap():
		case field.IsList():
			list := value.List()
			for i := 0; i < list.Len() && !found; i++ {
				found = valueHoldsHandle(field, list.Get(i))
			}
		default:
			found = valueHoldsHandle(field, value)
		}
		return !found
	})
	return found
}

func valueHoldsHandle(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
	switch field.Kind() {
	case protoreflect.StringKind:
		return sealedsecrets.ContainsHandle(value.String())
	case protoreflect.BytesKind:
		return sealedsecrets.ContainsHandle(string(value.Bytes()))
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return holdsHandle(value.Message())
	}
	return false
}
