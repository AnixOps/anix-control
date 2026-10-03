package nftables

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// item is one entry of an `nft -j list` answer: a table, chain, rule, set,
// map, counter, quota or another object, with its fields as nft printed
// them (numbers as json.Number).
type item struct {
	kind   string
	fields map[string]any
}

func (it item) str(key string) string {
	s, _ := it.fields[key].(string)
	return s
}

func (it item) num(key string) uint64 {
	switch v := it.fields[key].(type) {
	case json.Number:
		n, err := strconv.ParseUint(v.String(), 10, 64)
		if err == nil {
			return n
		}
	case string: // tc and some nft versions print numbers as strings
		n, err := strconv.ParseUint(v, 0, 64)
		if err == nil {
			return n
		}
	}
	return 0
}

func (it item) family() string { return it.str("family") }

// tableName answers the table an item belongs to ("" for a table).
func (it item) tableName() string {
	if it.kind == "table" {
		return it.str("name")
	}
	return it.str("table")
}

// listing is a parsed `nft -j list table|ruleset`.
type listing struct {
	items []item
}

// parseListing parses nft's JSON output.
func parseListing(b []byte) (*listing, error) {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("nftables driver: parse nft -j output: %w", err)
	}
	l := &listing{}
	for _, entry := range doc.Nftables {
		for kind, raw := range entry {
			if kind == "metainfo" {
				continue
			}
			var f map[string]any
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			if err := d.Decode(&f); err != nil {
				return nil, fmt.Errorf("nftables driver: parse nft -j %s: %w", kind, err)
			}
			l.items = append(l.items, item{kind: kind, fields: f})
		}
	}
	return l, nil
}

// table answers the table item of family and name, if listed.
func (l *listing) table(family, name string) (item, bool) {
	for _, it := range l.items {
		if it.kind == "table" && it.family() == family && it.str("name") == name {
			return it, true
		}
	}
	return item{}, false
}

// in answers the listing's items of one table.
func (l *listing) in(family, name string) *listing {
	out := &listing{}
	for _, it := range l.items {
		if it.family() == family && it.tableName() == name {
			out.items = append(out.items, it)
		}
	}
	return out
}

// object answers the named object of kind (counter, quota, set, map,
// chain).
func (l *listing) object(kind, name string) (item, bool) {
	for _, it := range l.items {
		if it.kind == kind && it.str("name") == name {
			return it, true
		}
	}
	return item{}, false
}

// elementComments answers the comments of a set's elements by their key
// value, for the driver's state sets.
func (it item) elementComments() map[uint64]string {
	out := map[uint64]string{}
	elems, _ := it.fields["elem"].([]any)
	for _, e := range elems {
		obj, ok := e.(map[string]any)
		if !ok {
			continue
		}
		inner, ok := obj["elem"].(map[string]any)
		if !ok {
			continue
		}
		val, ok := inner["val"].(json.Number)
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(val.String(), 10, 64)
		if err != nil {
			continue
		}
		c, _ := inner["comment"].(string)
		out[n] = c
	}
	return out
}

// flags answers a set's flags.
func (it item) flags() []string {
	var out []string
	switch v := it.fields["flags"].(type) {
	case []any:
		for _, f := range v {
			if s, ok := f.(string); ok {
				out = append(out, s)
			}
		}
	case string:
		out = append(out, v)
	}
	return out
}

// normalizeOptions say what normalized leaves out besides handles,
// counter values, quota usage, the elements of dynamic sets and the
// driver's state sets, which it always leaves out.
type normalizeOptions struct {
	// noComments drops the comments of counters (the counter epochs), so
	// two hosts running one artifact list the same objects.
	noComments bool
}

// normalized answers the owned table as stable strings in a stable order:
// one per object, sorted, and one per rule, in order within its chain.
// Values that change without an apply (counters, quota usage, connection
// counts) and the handles, which change when an object is re-created, are
// left out, and so are the elements of the state sets, which the driver
// rewrites without a full apply.
func (l *listing) normalized(o normalizeOptions) []string {
	type entry struct {
		kind, name string
		seq        int
		text       string
	}
	var out []entry
	ruleSeq := map[string]int{}
	for _, it := range l.items {
		f := make(map[string]any, len(it.fields))
		for k, v := range it.fields {
			f[k] = v
		}
		delete(f, "handle")
		name := it.str("name")
		switch it.kind {
		case "counter":
			delete(f, "packets")
			delete(f, "bytes")
			if o.noComments {
				delete(f, "comment")
			}
		case "quota":
			delete(f, "used")
		case "set", "map":
			if slices.Contains(it.flags(), "dynamic") || name == stateSet || name == sealSet {
				delete(f, "elem")
			}
		case "rule":
			name = it.str("chain")
			ruleSeq[name]++
		}
		b, err := json.Marshal(f)
		if err != nil {
			b = []byte(fmt.Sprintf("%v", f))
		}
		e := entry{kind: it.kind, name: name, text: it.kind + " " + string(b)}
		if it.kind == "rule" {
			e.seq = ruleSeq[name]
			e.text = fmt.Sprintf("rule %s #%d %s", name, e.seq, string(b))
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b entry) int {
		return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.name, b.name), cmp.Compare(a.seq, b.seq), cmp.Compare(a.text, b.text))
	})
	lines := make([]string, len(out))
	for i, e := range out {
		lines[i] = e.text
	}
	return lines
}

// fingerprint is the SHA-256 of the normalized listing, comments kept: it
// changes when anything Apply wrote changes, and not with traffic.
func (l *listing) fingerprint() string {
	sum := sha256.Sum256([]byte(strings.Join(l.normalized(normalizeOptions{}), "\n")))
	return hex.EncodeToString(sum[:])
}
