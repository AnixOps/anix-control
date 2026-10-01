package sealedsecrets

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	configtables "github.com/AnixOps/anix-control/v4/config"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
)

// Placeholder is the masked value of a node secret. Sent back, it keeps
// the stored secret, so it passes unchanged.
const Placeholder = nodesecrets.Placeholder

// Why a substitution or an expansion failed. They are the complete, fixed
// set of reasons the gateway counts.
var (
	ErrFieldsUnavailable = errors.New("the node secret field list is unavailable")
	ErrNotJSON           = errors.New("the request body is not the JSON the route takes")
	ErrValueNotString    = errors.New("a secret field holds a value that is not a string")
	ErrTargetInvalid     = errors.New("the route's target is not a resource id")
	ErrTooLarge          = errors.New("the sealed request exceeds the request body limit")
	ErrAnswerHandle      = errors.New("the answer holds a sealed handle it cannot show")
	ErrStore             = errors.New("the sealed secret store failed")
)

// Sealer seals the gateway's requests and expands its answers for the
// routes of a field list.
type Sealer struct {
	table    *Table
	tableErr error
	store    *Store
}

// NewSealer returns a sealer for table on store. A nil table with err
// fails closed: every request with a body is refused.
func NewSealer(table *Table, err error, store *Store) *Sealer {
	if table == nil && err == nil {
		err = ErrFieldsUnavailable
	}
	if store == nil {
		store = DefaultStore()
	}
	return &Sealer{table: table, tableErr: err, store: store}
}

var (
	defaultSealerOnce sync.Once
	defaultSealer     *Sealer
)

// DefaultSealer is the process's sealer: the field list the kernel binary
// embeds, on DefaultStore.
func DefaultSealer() *Sealer {
	defaultSealerOnce.Do(func() {
		table, err := ParseTable(configtables.NodeSecretFields)
		defaultSealer = NewSealer(table, err, DefaultStore())
	})
	return defaultSealer
}

// Store returns the sealer's store.
func (s *Sealer) Store() *Store {
	if s == nil {
		return nil
	}
	return s.store
}

// Table returns the sealer's field list, nil when it is unavailable.
func (s *Sealer) Table() *Table {
	if s == nil {
		return nil
	}
	return s.table
}

// Listed reports whether a route's requests are sealed: it is in the field
// list, or the list is unavailable and nothing can be told.
func (s *Sealer) Listed(routeID string) bool {
	if s == nil {
		return false
	}
	if s.table == nil {
		return true
	}
	_, ok := s.table.Route(routeID)
	return ok
}

// RequestInput is one gateway request of a listed route.
type RequestInput struct {
	PackageID  string
	Generation uint64
	RequestID  string
	RouteID    string
	Deadline   time.Time
	Body       []byte
	PathParams map[string]string
	// BodyLimit bounds the sealed body, as the gateway bounds the request
	// body; zero leaves it unbounded.
	BodyLimit int64
}

// Sealed is a request sealed for its package host.
type Sealed struct {
	// Key names the sealed request; the bridge capability carries it, and
	// Release ends it.
	Key string
	// Body is what the package host reads; the kernel's legacy handler
	// reads the original.
	Body []byte
	// Count is the number of secrets sealed.
	Count int
}

// SealRequest replaces every secret of a listed route's request body with a
// handle bound to the request. It fails closed: a body that is not JSON, a
// secret field holding a value that is not a string, a target path
// parameter that is not an id, a sealed body over the limit, or an
// unavailable field list is an error, and the caller serves the request
// without the package host or refuses it. On success the caller releases
// the key when the request ends, with or without an error.
func (s *Sealer) SealRequest(input RequestInput) (Sealed, error) {
	if s == nil {
		return Sealed{}, ErrFieldsUnavailable
	}
	if s.table == nil {
		if len(bytes.TrimSpace(input.Body)) != 0 {
			return Sealed{}, ErrFieldsUnavailable
		}
		key, err := s.store.open(Request{
			PackageID: input.PackageID, Generation: input.Generation, RequestID: input.RequestID, RouteID: input.RouteID,
			Target: Target{Kind: TargetNone}, Deadline: input.Deadline,
		}, nil)
		if err != nil {
			return Sealed{}, ErrStore
		}
		return Sealed{Key: key, Body: input.Body}, nil
	}
	route, ok := s.table.Route(input.RouteID)
	if !ok {
		return Sealed{Body: input.Body}, nil
	}
	request := Request{
		PackageID: input.PackageID, Generation: input.Generation, RequestID: input.RequestID, RouteID: input.RouteID,
		Target: Target{Kind: route.Target.Kind}, New: route.Target.New, Deadline: input.Deadline,
	}
	if route.Target.PathParam != "" {
		id, err := strconv.ParseUint(strings.TrimSpace(input.PathParams[route.Target.PathParam]), 10, 64)
		if err != nil || id == 0 {
			return Sealed{}, ErrTargetInvalid
		}
		request.Target.ID = id
	}
	key, err := s.store.open(request, route)
	if err != nil {
		return Sealed{}, ErrStore
	}
	sealed, count, err := s.sealBody(key, route, input.Body)
	if err == nil && input.BodyLimit > 0 && int64(len(sealed)) > input.BodyLimit {
		err = ErrTooLarge
	}
	if err != nil {
		s.store.Release(key)
		return Sealed{}, err
	}
	return Sealed{Key: key, Body: sealed, Count: count}, nil
}

// Release ends a sealed request.
func (s *Sealer) Release(key string) {
	if s != nil {
		s.store.Release(key)
	}
}

// bodySealer collects the edits of one request body.
type bodySealer struct {
	store *Store
	key   string
	route *Route
	count int
}

func (s *Sealer) sealBody(key string, route *Route, body []byte) ([]byte, int, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return body, 0, nil
	}
	root, err := parseSpans(body)
	if err != nil {
		return nil, 0, ErrNotJSON
	}
	sealer := &bodySealer{store: s.store, key: key, route: route}
	var edits []edit
	if err := sealer.walk(body, root, nil, &edits); err != nil {
		return nil, 0, err
	}
	return applyEdits(body, edits), sealer.count, nil
}

// walk seals the body: a listed field by its kind, and anywhere else every
// value under a key that nodesecrets.IsSecretKey marks, as the
// administrator's answers mask it. Objects under a secret key are searched
// instead of sealed whole.
func (b *bodySealer) walk(source []byte, node *span, path []string, edits *[]edit) error {
	switch node.kind {
	case kindObject:
		for index, key := range node.keys {
			item := node.items[index]
			childPath := append(append([]string(nil), path...), key)
			if field, listed := b.route.requestField(childPath); listed {
				if err := b.sealField(source, item, field, edits); err != nil {
					return err
				}
				continue
			}
			if item.kind != kindObject && nodesecrets.IsSecretKey(key) && hasSecret(item) {
				if err := b.sealSpan(source, item, joinPointer(childPath), edits); err != nil {
					return err
				}
				continue
			}
			if err := b.walk(source, item, childPath, edits); err != nil {
				return err
			}
		}
	case kindArray:
		for index, item := range node.items {
			if err := b.walk(source, item, append(append([]string(nil), path...), strconv.Itoa(index)), edits); err != nil {
				return err
			}
		}
	}
	return nil
}

// sealField seals one listed field.
func (b *bodySealer) sealField(source []byte, node *span, field RequestField, edits *[]edit) error {
	switch field.Kind {
	case FieldValue:
		switch node.kind {
		case kindNull:
			return nil
		case kindString:
			if node.text == "" || node.text == Placeholder {
				return nil
			}
			return b.sealSpan(source, node, field.Pointer, edits)
		default:
			return ErrValueNotString
		}
	case FieldDocument:
		switch node.kind {
		case kindObject, kindArray:
			return b.sealDocument(source, node, field.tokens, edits)
		case kindString:
			if strings.TrimSpace(node.text) == "" || node.text == Placeholder {
				return nil
			}
			document := []byte(node.text)
			inner, err := parseSpans(document)
			if err != nil {
				return ErrNotJSON
			}
			var innerEdits []edit
			if err := b.sealDocument(document, inner, field.tokens, &innerEdits); err != nil {
				return err
			}
			if len(innerEdits) > 0 {
				*edits = append(*edits, edit{start: node.start, end: node.end, text: encodeString(string(applyEdits(document, innerEdits)))})
			}
			return nil
		default:
			return nil
		}
	}
	return ErrFieldsUnavailable
}

// sealDocument seals a document's secrets. prefix is the listed field's
// pointer, as the field list spells it; the document's own path follows.
func (b *bodySealer) sealDocument(source []byte, node *span, prefix []string, edits *[]edit) error {
	switch node.kind {
	case kindObject:
		for index, key := range node.keys {
			item := node.items[index]
			childPath := append(append([]string(nil), prefix...), key)
			if item.kind != kindObject && nodesecrets.IsSecretKey(key) && hasSecret(item) {
				if err := b.sealSpan(source, item, joinPointer(childPath), edits); err != nil {
					return err
				}
				continue
			}
			if err := b.sealDocument(source, item, childPath, edits); err != nil {
				return err
			}
		}
	case kindArray:
		for index, item := range node.items {
			if err := b.sealDocument(source, item, append(append([]string(nil), prefix...), strconv.Itoa(index)), edits); err != nil {
				return err
			}
		}
	}
	return nil
}

// hasSecret reports whether a value under a secret key is sealed: a string
// that is neither empty nor the placeholder (KernelNodeOps refuses any other
// string in clear there), or a non-empty array.
func hasSecret(node *span) bool {
	switch node.kind {
	case kindString:
		return node.text != "" && node.text != Placeholder
	case kindArray:
		return len(node.items) > 0
	}
	return false
}

// sealSpan replaces one value with a handle: a string by its value, any
// other value by its JSON encoding.
func (b *bodySealer) sealSpan(source []byte, node *span, field string, edits *[]edit) error {
	secret := Secret{Value: node.text}
	if node.kind != kindString {
		secret = Secret{Value: string(source[node.start:node.end]), JSON: true}
	}
	handle, err := b.store.seal(b.key, field, secret)
	if err != nil {
		return ErrStore
	}
	b.count++
	*edits = append(*edits, edit{start: node.start, end: node.end, text: encodeString(handle)})
	return nil
}

// Header is one answer header.
type Header struct {
	Name  string
	Value string
}

// ExpandAnswer returns a package's answer as the client gets it: every
// handle the kernel minted for this request (key) under the name of the
// listed answer field it stands in is replaced by its secret, once. Any
// other handle that would reach the client fails it: a handle at a listed
// answer field that is not one of this request's, or a handle sealed or
// minted for any request in flight, anywhere in the body or the headers.
// A string at an unlisted position that has the shape of a handle but is
// none is data and passes. It returns the answer and how many handles it
// expanded.
func (s *Sealer) ExpandAnswer(key, routeID string, body []byte, headers []Header) ([]byte, int, error) {
	if s == nil {
		return body, 0, nil
	}
	for _, header := range headers {
		if s.holdsLiveHandle(header.Name) || s.holdsLiveHandle(header.Value) {
			return nil, 0, ErrAnswerHandle
		}
	}
	if !mayHoldHandle(body) {
		return body, 0, nil
	}
	var route *Route
	if s.table != nil {
		route, _ = s.table.Route(routeID)
	}
	root, err := parseSpans(body)
	if err != nil {
		if s.holdsLiveHandle(string(body)) {
			return nil, 0, ErrAnswerHandle
		}
		return body, 0, nil
	}
	expander := answerExpander{sealer: s, key: key, route: route}
	var edits []edit
	if err := expander.walk(root, nil, &edits); err != nil {
		return nil, 0, err
	}
	return applyEdits(body, edits), expander.count, nil
}

type answerExpander struct {
	sealer *Sealer
	key    string
	route  *Route
	count  int
}

func (a *answerExpander) walk(node *span, path []string, edits *[]edit) error {
	switch node.kind {
	case kindObject:
		for index, key := range node.keys {
			if a.sealer.holdsLiveHandle(key) {
				return ErrAnswerHandle
			}
			if err := a.walk(node.items[index], append(append([]string(nil), path...), key), edits); err != nil {
				return err
			}
		}
	case kindArray:
		for index, item := range node.items {
			if err := a.walk(item, append(append([]string(nil), path...), strconv.Itoa(index)), edits); err != nil {
				return err
			}
		}
	case kindString:
		if !strings.Contains(node.text, Prefix) {
			return nil
		}
		if field, listed := a.route.answerField(path); listed && IsHandle(node.text) {
			if a.key == "" {
				return ErrAnswerHandle
			}
			secret, ok := a.sealer.store.expand(a.key, node.text, field.Name)
			if !ok {
				return ErrAnswerHandle
			}
			a.count++
			*edits = append(*edits, edit{start: node.start, end: node.end, text: encodeString(secret)})
			return nil
		}
		if a.sealer.holdsLiveHandle(node.text) {
			return ErrAnswerHandle
		}
	}
	return nil
}

// holdsLiveHandle reports whether text holds a handle of a request in
// flight.
func (s *Sealer) holdsLiveHandle(text string) bool {
	return findHandle(text, s.store.Live)
}

// mayHoldHandle reports whether a body may hold a handle: the prefix as
// written, or a character of it written as a \u escape, which JSON
// encoders never do on their own. Most answers hold neither and are passed
// without being parsed.
func mayHoldHandle(body []byte) bool {
	if bytes.Contains(body, []byte(Prefix)) {
		return true
	}
	escape := []byte(`\u00`)
	for offset := 0; ; {
		index := bytes.Index(body[offset:], escape)
		if index < 0 {
			return false
		}
		start := offset + index
		if start+6 <= len(body) {
			if value, err := strconv.ParseUint(string(body[start+4:start+6]), 16, 8); err == nil && strings.IndexByte(Prefix, byte(value)) >= 0 {
				return true
			}
		}
		offset = start + 1
	}
}
