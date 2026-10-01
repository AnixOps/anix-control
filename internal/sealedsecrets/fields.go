package sealedsecrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// FieldsFormat is the format of config/node-secret-fields.json.
const FieldsFormat = "anixops.node-secret-fields/v1"

// Target kinds: what a route's sealed secrets may be stored for.
const (
	TargetProxy           = "proxy"
	TargetForward         = "forward"
	TargetProtocol        = "protocol"
	TargetCleanAgent      = "clean_agent"
	TargetRegistrationKey = "registration_key"
	// TargetNone is a route whose secrets are never stored (validation):
	// its handles never resolve.
	TargetNone = "none"
	// TargetDial is a route whose secret is presented once, by the kernel,
	// to the address the request names, and never stored (a connection
	// test): its handles resolve for the bound request only, for no
	// resource.
	TargetDial = "dial"
)

// Request field kinds.
const (
	// FieldValue is one secret value: a string. The placeholder and an
	// empty string pass unchanged; any other non-string value fails the
	// substitution, so the legacy handler answers it.
	FieldValue = "value"
	// FieldDocument is a JSON document (a protocol setting, a raw
	// configuration), inline or as a JSON string: every value under a key
	// that nodesecrets.IsSecretKey marks is sealed, as the administrator's
	// answers mask it.
	FieldDocument = "document"
)

// Table is the kernel-owned list of the v2 routes whose requests or answers
// carry node secrets, and where (config/node-secret-fields.json).
type Table struct {
	routes map[string]*Route
}

// Route is one listed route.
type Route struct {
	ID      string
	Target  TargetRule
	Request []RequestField
	Answer  []AnswerField
}

// TargetRule says what the route's sealed secrets may be stored for: the
// resource named by a path parameter, a resource the request creates, or
// nothing.
type TargetRule struct {
	Kind      string
	PathParam string
	New       bool
}

// RequestField is a request position that carries a secret an
// administrator types.
type RequestField struct {
	Pointer string
	Kind    string
	tokens  []string
}

// AnswerField is an answer position where a secret is shown once.
type AnswerField struct {
	Pointer string
	// Name names the secret, as SecretHandle.field does: only a handle the
	// kernel minted under this name expands here.
	Name   string
	tokens []string
}

type fieldsDocument struct {
	Format string           `json:"format"`
	Routes []routeStatement `json:"routes"`
}

type routeStatement struct {
	RouteID string `json:"route_id"`
	Target  struct {
		Kind      string `json:"kind"`
		PathParam string `json:"path_param"`
		New       bool   `json:"new"`
	} `json:"target"`
	Request []struct {
		Pointer string `json:"pointer"`
		Kind    string `json:"kind"`
	} `json:"request"`
	Answer []struct {
		Pointer string `json:"pointer"`
		Name    string `json:"name"`
	} `json:"answer"`
}

// ParseTable reads and checks a field list. The route gate
// (config/scripts/check_plugin_only_routes.py) also checks every route id
// against config/package-extraction.json.
func ParseTable(raw []byte) (*Table, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document fieldsDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("node secret fields: %w", err)
	}
	if decoder.More() {
		return nil, errors.New("node secret fields: one document expected")
	}
	if document.Format != FieldsFormat {
		return nil, fmt.Errorf("node secret fields: format must be %s", FieldsFormat)
	}
	table := &Table{routes: make(map[string]*Route, len(document.Routes))}
	for _, statement := range document.Routes {
		route, err := statement.route()
		if err != nil {
			return nil, fmt.Errorf("node secret fields: route %q: %w", statement.RouteID, err)
		}
		if _, duplicate := table.routes[route.ID]; duplicate {
			return nil, fmt.Errorf("node secret fields: route %q is listed twice", route.ID)
		}
		table.routes[route.ID] = route
	}
	return table, nil
}

func (s routeStatement) route() (*Route, error) {
	if strings.TrimSpace(s.RouteID) == "" {
		return nil, errors.New("route_id is required")
	}
	route := &Route{ID: s.RouteID, Target: TargetRule{Kind: s.Target.Kind, PathParam: s.Target.PathParam, New: s.Target.New}}
	switch s.Target.Kind {
	case TargetProxy, TargetForward, TargetProtocol, TargetCleanAgent, TargetRegistrationKey:
		if (s.Target.PathParam == "") == !s.Target.New {
			return nil, errors.New("the target names a path parameter or is new, not both")
		}
	case TargetNone, TargetDial:
		if s.Target.PathParam != "" || s.Target.New {
			return nil, fmt.Errorf("a target of kind %s has no path parameter", s.Target.Kind)
		}
	default:
		return nil, fmt.Errorf("target kind %q is unknown", s.Target.Kind)
	}
	if len(s.Request) == 0 && len(s.Answer) == 0 {
		return nil, errors.New("a route lists request or answer fields")
	}
	seen := map[string]bool{}
	for _, field := range s.Request {
		tokens, err := splitPointer(field.Pointer)
		if err != nil || len(tokens) == 0 {
			return nil, fmt.Errorf("request pointer %q is not a field", field.Pointer)
		}
		folded := foldPointer(tokens)
		if seen[folded] {
			return nil, fmt.Errorf("request pointer %q is listed twice", field.Pointer)
		}
		seen[folded] = true
		if field.Kind != FieldValue && field.Kind != FieldDocument {
			return nil, fmt.Errorf("request field kind %q is unknown", field.Kind)
		}
		route.Request = append(route.Request, RequestField{Pointer: field.Pointer, Kind: field.Kind, tokens: tokens})
	}
	names := map[string]bool{}
	pointers := map[string]bool{}
	for _, field := range s.Answer {
		tokens, err := splitPointer(field.Pointer)
		if err != nil || len(tokens) == 0 {
			return nil, fmt.Errorf("answer pointer %q is not a field", field.Pointer)
		}
		if strings.TrimSpace(field.Name) == "" || names[field.Name] || pointers[field.Pointer] {
			return nil, fmt.Errorf("answer field %q needs a name of its own", field.Pointer)
		}
		names[field.Name], pointers[field.Pointer] = true, true
		route.Answer = append(route.Answer, AnswerField{Pointer: field.Pointer, Name: field.Name, tokens: tokens})
	}
	return route, nil
}

// Route returns a listed route.
func (t *Table) Route(routeID string) (*Route, bool) {
	if t == nil {
		return nil, false
	}
	route, ok := t.routes[routeID]
	return route, ok
}

// RouteIDs returns the listed route ids.
func (t *Table) RouteIDs() []string {
	if t == nil {
		return nil
	}
	ids := make([]string, 0, len(t.routes))
	for id := range t.routes {
		ids = append(ids, id)
	}
	return ids
}

// answerField returns the answer field at an exact answer path.
func (r *Route) answerField(path []string) (AnswerField, bool) {
	if r == nil {
		return AnswerField{}, false
	}
	for _, field := range r.Answer {
		if equalTokens(field.tokens, path) {
			return field, true
		}
	}
	return AnswerField{}, false
}

// answerName reports whether the route shows a secret of that name.
func (r *Route) answerName(name string) bool {
	if r == nil {
		return false
	}
	for _, field := range r.Answer {
		if field.Name == name {
			return true
		}
	}
	return false
}

// requestField returns the request field a request path names. Member
// names match as the legacy handlers bind them, whatever their case or
// underscores (encoding/json folds case, and the update maps resolve
// columns without underscores), so a field cannot pass under another
// spelling.
func (r *Route) requestField(path []string) (RequestField, bool) {
	if r == nil {
		return RequestField{}, false
	}
	folded := foldPointer(path)
	for _, field := range r.Request {
		if foldPointer(field.tokens) == folded {
			return field, true
		}
	}
	return RequestField{}, false
}

func equalTokens(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func foldPointer(tokens []string) string {
	folded := make([]string, len(tokens))
	for index, token := range tokens {
		folded[index] = foldKey(token)
	}
	return strings.Join(folded, "\x00")
}

// foldKey is a member name as compared with a listed field: every rune
// folded to the least rune of its case-folding orbit (so the matching is
// that of strings.EqualFold, which encoding/json uses), without the
// separators _, -, . and space.
func foldKey(key string) string {
	var builder strings.Builder
	for _, character := range key {
		switch character {
		case '_', '-', '.', ' ':
			continue
		}
		least := character
		for folded := unicode.SimpleFold(character); folded != character; folded = unicode.SimpleFold(folded) {
			if folded < least {
				least = folded
			}
		}
		builder.WriteRune(least)
	}
	return builder.String()
}
