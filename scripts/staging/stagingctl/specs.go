package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Persona keys. Every persona except anon is a seeded account
// (World.Personas); admin is the bootstrap super administrator.
const (
	Anon    = "anon"    // no token
	Admin   = "admin"   // super administrator (is_admin, not staff)
	Admin2  = "admin2"  // a second administrator
	Staff   = "staff"   // is_admin and is_staff
	User    = "user"    // a member with a plan, orders, tickets, invites ...
	User2   = "user2"   // another member: other members' resources
	Fresh   = "fresh"   // a member without any data: empty results
	Banned  = "banned"  // a banned member
	Expired = "expired" // a member whose plan expired
)

// Req is one request of the replayer.
type Req struct {
	Persona string
	Path    string     // concrete path, path parameters filled in
	Query   url.Values // nil for none
	Body    any        // nil, []byte, string (raw JSON) or a value to marshal
	Label   string     // what the variant exercises: "page 2", "not found", "forbidden" ...
	// Mask lists dotted JSON paths of the response that legitimately
	// differ between the two reconciliation twins (clock values, random
	// tokens); "*" matches every array element or object key.
	Mask []string
}

// BodyBytes encodes the request body.
func (r Req) BodyBytes() []byte {
	switch body := r.Body.(type) {
	case nil:
		return nil
	case []byte:
		return body
	case string:
		return []byte(body)
	default:
		data, err := json.Marshal(body)
		if err != nil {
			panic(fmt.Sprintf("encode body of %s: %v", r.Path, err))
		}
		return data
	}
}

// RouteSpec generates traffic for one native-flagged route.
type RouteSpec struct {
	RouteID string
	// Reads returns the request variants of a GET route; the replayer
	// cycles through them. Include pagination, filters, empty results,
	// not found, invalid input and permission errors.
	Reads func(w *World) []Req
	// Writes returns the ordered write requests of a non-GET route for the
	// reconciliation twins. Each request runs once on each twin, in the
	// order of the batch's routes and then of this list; use seeded ids
	// (World.IDs) so both twins act on the same rows. Include negative
	// cases. Leave out requests whose effect leaves the staging network
	// (they would fail on both twins the same way anyway).
	Writes func(w *World) []Req
	// Skip, when set, explains why the route gets no traffic (the report
	// lists it as not rehearsed and the batch cannot pass while it is set).
	Skip string
}

// TableSpec is one table compared row by row after the write replay.
type TableSpec struct {
	Table string
	// Key orders the rows (a column list, default "id").
	Key string
	// Ignore lists columns whose values are expected to differ between
	// the twins, each with the reason (documented in the report).
	Ignore map[string]string
	// Where optionally limits the compared rows (an SQL condition).
	Where string
}

var (
	routeSpecs = map[string]RouteSpec{}
	// reconTables lists, per package, the tables its write routes change.
	reconTables = map[string][]TableSpec{}
)

func registerSpecs(specs ...RouteSpec) {
	for _, spec := range specs {
		if _, ok := routeSpecs[spec.RouteID]; ok {
			panic("duplicate route spec " + spec.RouteID)
		}
		if spec.Reads == nil && spec.Writes == nil && spec.Skip == "" {
			panic("route spec " + spec.RouteID + " has no traffic and no skip reason")
		}
		routeSpecs[spec.RouteID] = spec
	}
}

func registerTables(packageID string, tables ...TableSpec) {
	reconTables[packageID] = append(reconTables[packageID], tables...)
}

// timestamps is the usual ignore list: the handlers stamp rows from their
// own clock, so the twins never agree on them.
func timestamps(extra ...string) map[string]string {
	ignore := map[string]string{
		"created_at": "set from the handler's clock",
		"updated_at": "set from the handler's clock",
	}
	for i := 0; i+1 < len(extra); i += 2 {
		ignore[extra[i]] = extra[i+1]
	}
	return ignore
}

// specCoverage returns the batch's routes without a spec.
func specCoverage(routes []CatalogRoute) []string {
	var missing []string
	for _, route := range routes {
		if _, ok := routeSpecs[route.RouteID]; !ok {
			missing = append(missing, route.RouteID)
		}
	}
	sort.Strings(missing)
	return missing
}

// fill substitutes :name path parameters.
func fill(pattern string, values ...any) string {
	parts := strings.Split(pattern, "/")
	index := 0
	for i, part := range parts {
		if strings.HasPrefix(part, ":") && index < len(values) {
			parts[i] = fmt.Sprint(values[index])
			index++
		}
	}
	return strings.Join(parts, "/")
}

// q builds query values from key, value pairs.
func q(pairs ...string) url.Values {
	values := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values.Add(pairs[i], pairs[i+1])
	}
	return values
}

// forbidden returns the permission and authentication variants of an admin
// route: a member and an anonymous caller.
func forbidden(path string) []Req {
	return []Req{
		{Persona: User, Path: path, Label: "member on admin route"},
		{Persona: Anon, Path: path, Label: "anonymous"},
	}
}
