// Package sealedsecrets keeps node secrets away from package hosts
// (docs/architecture/node-ops-service.md section 3.7, NO-4).
//
// For the routes and fields of config/node-secret-fields.json the kernel's
// v2 gateway replaces every secret an administrator sends with an opaque,
// single-use handle before the package host reads the request, and expands
// the handles the kernel minted for a secret shown once only in the answer
// to the request they were minted for. A handle lives in kernel memory
// only, bound to one request (its key, package generation, request id and
// route), its target and its field, and dies with the request. Handles and
// the secrets they stand for never appear in logs, metric labels, the
// KernelNodeOps ledger or error texts.
package sealedsecrets

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/v2compat"
)

// Prefix starts every handle (v2compat.SealedHandlePrefix).
const Prefix = v2compat.SealedHandlePrefix

// handleBytes is the randomness of a handle and of a request key.
const handleBytes = 32

var (
	// ErrRefused is every refused resolution, mint or expansion. Its text
	// names no handle, value, request or field.
	ErrRefused = errors.New("sealed secret handle refused")
	// ErrRequestEnded is a binding whose request has ended or was never
	// sealed.
	ErrRequestEnded = errors.New("the sealed request has ended")
)

// IsHandle reports whether value is a sealed handle.
func IsHandle(value string) bool { return v2compat.IsSealedHandle(value) }

// ContainsHandle reports whether text holds a sealed handle anywhere.
func ContainsHandle(text string) bool {
	return findHandle(text, func(string) bool { return true })
}

// findHandle reports whether text holds a handle that match accepts.
func findHandle(text string, match func(string) bool) bool {
	for offset := 0; ; {
		index := strings.Index(text[offset:], Prefix)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + v2compat.SealedHandleLength
		if end <= len(text) && IsHandle(text[start:end]) && match(text[start:end]) {
			return true
		}
		offset = start + len(Prefix)
	}
}

// Direction is where a handle came from.
type Direction uint8

const (
	// Inbound handles stand for secrets an administrator's request carried.
	Inbound Direction = iota + 1
	// Outbound handles stand for secrets the kernel generated for the
	// request's answer.
	Outbound
)

// Target is what a sealed secret is stored for: a kind of
// config/node-secret-fields.json and a resource id.
type Target struct {
	Kind string
	ID   uint64
}

// Request is one gateway request whose secrets are sealed.
type Request struct {
	PackageID  string
	Generation uint64
	RequestID  string
	RouteID    string
	// Target is the route's target for this request: ID 0 with New for a
	// resource the request creates (the first resolution binds it), or
	// kind none.
	Target   Target
	New      bool
	Deadline time.Time
}

// Binding is a verified request binding as a KernelNodeOps executor holds
// it: the live request the package is serving.
type Binding struct {
	Key        string
	PackageID  string
	Generation uint64
	RequestID  string
	RouteID    string
}

// Use is one resolution an executor asks for: the handle, and the target
// and field the secret is stored for.
type Use struct {
	Handle string
	Target Target
	// Field is the request position the handle was sealed from: the listed
	// pointer as config/node-secret-fields.json spells it, followed, in a
	// document, by the secret's pointer inside the document (for example
	// "/reality_settings/private_key").
	Field string
}

// Secret is a resolved value.
type Secret struct {
	// Value is the secret: a string, or with JSON set the JSON encoding of
	// a non-string value (an array under a secret key).
	Value string
	JSON  bool
}

type sealedRequest struct {
	key      string
	request  Request
	route    *Route
	handles  []string
	bound    uint64
	released bool
}

type entry struct {
	request   *sealedRequest
	direction Direction
	field     string
	name      string
	secret    Secret
	used      bool
}

// Store holds the handles of the requests in flight. The zero value is not
// usable; use NewStore.
type Store struct {
	mu       sync.Mutex
	requests map[string]*sealedRequest
	handles  map[string]*entry
	now      func() time.Time
}

// NewStore returns an empty store. now defaults to time.Now.
func NewStore(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{requests: map[string]*sealedRequest{}, handles: map[string]*entry{}, now: now}
}

var defaultStore = NewStore(nil)

// DefaultStore is the process's store: the gateway seals into it and the
// KernelNodeOps engine resolves from it.
func DefaultStore() *Store { return defaultStore }

func randomToken() (string, error) {
	var raw [handleBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// open registers a request and returns its key. Requests past their
// deadline are dropped on the way.
func (s *Store) open(request Request, route *Route) (string, error) {
	key, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	s.requests[key] = &sealedRequest{key: key, request: request, route: route}
	return key, nil
}

// purgeLocked drops the requests whose deadline passed: a request the
// gateway did not release (it never fails to) still dies with its deadline.
func (s *Store) purgeLocked() {
	now := s.now()
	for key, request := range s.requests {
		if !now.Before(request.request.Deadline) {
			s.releaseLocked(key)
		}
	}
}

// seal stores one inbound secret for an open request and returns its
// handle.
func (s *Store) seal(key, field string, secret Secret) (string, error) {
	return s.add(key, &entry{direction: Inbound, field: field, secret: secret})
}

func (s *Store) add(key string, item *entry) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	handle := Prefix + token
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.requests[key]
	if !ok || request.released || !s.now().Before(request.request.Deadline) {
		return "", ErrRequestEnded
	}
	item.request = request
	s.handles[handle] = item
	request.handles = append(request.handles, handle)
	return handle, nil
}

// Release ends a request: every handle sealed or minted for it is dropped
// with its secret. Releasing twice is harmless.
func (s *Store) Release(key string) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked(key)
}

func (s *Store) releaseLocked(key string) {
	request, ok := s.requests[key]
	if !ok {
		return
	}
	for _, handle := range request.handles {
		if item := s.handles[handle]; item != nil {
			item.secret = Secret{}
		}
		delete(s.handles, handle)
	}
	request.handles, request.released = nil, true
	delete(s.requests, key)
}

// live returns the open request a binding names: the same key, package
// generation, request id and route, before its deadline.
func (s *Store) liveLocked(binding Binding) (*sealedRequest, error) {
	request, ok := s.requests[binding.Key]
	if !ok || request.released || !s.now().Before(request.request.Deadline) {
		return nil, ErrRequestEnded
	}
	if request.request.PackageID != binding.PackageID || request.request.Generation != binding.Generation ||
		request.request.RequestID != binding.RequestID || request.request.RouteID != binding.RouteID {
		return nil, ErrRefused
	}
	return request, nil
}

// Resolve resolves inbound handles for the request a binding names, all or
// none, and each once: a handle sealed from another request, route, target
// or field, one used before, one the kernel minted for an answer, or one of
// a route that stores no secret (target kind none) is refused, and nothing
// is used. A request that creates its target binds it at its first
// resolution: every later one must name the same resource. A dial target
// names no resource: a use names the kind and no id.
func (s *Store) Resolve(binding Binding, uses ...Use) ([]Secret, error) {
	if s == nil || len(uses) == 0 {
		return nil, ErrRefused
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	request, err := s.liveLocked(binding)
	if err != nil {
		return nil, err
	}
	target := request.request.Target
	if target.Kind == TargetNone || target.Kind == "" {
		return nil, ErrRefused
	}
	bound := target.ID
	if request.request.New {
		bound = request.bound
	}
	items := make([]*entry, len(uses))
	seen := make(map[string]bool, len(uses))
	for index, use := range uses {
		item := s.handles[use.Handle]
		if item == nil || seen[use.Handle] || item.request != request || item.direction != Inbound || item.used ||
			item.field != use.Field || use.Target.Kind != target.Kind {
			return nil, ErrRefused
		}
		if target.Kind == TargetDial {
			if use.Target.ID != 0 {
				return nil, ErrRefused
			}
		} else if use.Target.ID == 0 || (bound != 0 && use.Target.ID != bound) {
			return nil, ErrRefused
		}
		if bound == 0 {
			bound = use.Target.ID
		}
		seen[use.Handle], items[index] = true, item
	}
	secrets := make([]Secret, len(items))
	for index, item := range items {
		item.used, secrets[index] = true, item.secret
	}
	if request.request.New {
		request.bound = bound
	}
	return secrets, nil
}

// Mint stores a secret the kernel generated for the answer to the request a
// binding names, under a name the route's answer shows (SecretHandle.field),
// and returns its handle and when it expires. The gateway expands it in
// that answer only.
func (s *Store) Mint(binding Binding, name, value string) (string, time.Time, error) {
	if s == nil || value == "" {
		return "", time.Time{}, ErrRefused
	}
	s.mu.Lock()
	request, err := s.liveLocked(binding)
	if err == nil && !request.route.answerName(name) {
		err = ErrRefused
	}
	var deadline time.Time
	if err == nil {
		deadline = request.request.Deadline
	}
	s.mu.Unlock()
	if err != nil {
		return "", time.Time{}, err
	}
	handle, err := s.add(binding.Key, &entry{direction: Outbound, name: name, secret: Secret{Value: value}})
	if err != nil {
		return "", time.Time{}, err
	}
	return handle, deadline, nil
}

// Live reports whether a handle is in the store: sealed or minted for a
// request in flight.
func (s *Store) Live(handle string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.handles[handle]
	return ok
}

// expand returns the secret of an outbound handle minted for the request
// key under name, once.
func (s *Store) expand(key, handle, name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.handles[handle]
	if item == nil || item.request.key != key || item.request.released || item.direction != Outbound ||
		item.used || item.name != name || !s.now().Before(item.request.request.Deadline) {
		return "", false
	}
	item.used = true
	return item.secret.Value, true
}

// Pending reports how many requests the store holds, for tests and
// diagnostics.
func (s *Store) Pending() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}
