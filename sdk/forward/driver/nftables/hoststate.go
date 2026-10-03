package nftables

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// What the driver records on the host, inside its own table, so a new
// driver instance (an Agent restart) finds it and so it changes in the same
// transaction as the rules it describes:
//
//   - set anixops_state: the state document (hostDoc), JSON encoded in
//     base64url and cut into chunks of stateChunk characters, each the
//     comment of one element whose key is the chunk's index. nft cannot
//     change a table's comment in place and limits comments to 128
//     characters; set elements can be flushed and re-added in the same
//     transaction as everything else.
//   - set anixops_seal: one element whose comment is the fingerprint of the
//     table as the last Apply or SetUpstreams left it (listing.fingerprint),
//     written right after that transaction. Apply compares it with the
//     table it finds: a damaged or partial table no longer matches, and is
//     repaired.
//   - the comments of the hops' named counters: "anixops epoch <nonce>",
//     set when the counter is created and never changed (nft ignores the
//     comment of an existing object), so the counter epoch ends exactly
//     when the counter is re-created.
const (
	stateSet   = "anixops_state"
	sealSet    = "anixops_seal"
	stateChunk = 120
	epochNote  = "anixops epoch "
)

// hostDoc is the state document.
type hostDoc struct {
	V      int       `json:"v"`
	Node   string    `json:"node,omitempty"`
	Gen    uint64    `json:"gen"`
	Hash   string    `json:"hash,omitempty"`
	Digest string    `json:"digest"`
	Hops   []*docHop `json:"hops,omitempty"`
}

// docHop is one applied hop: what SetUpstreams and Observe need.
type docHop struct {
	Route   string                    `json:"r"`
	Index   uint32                    `json:"i"`
	Balance forwardv1.BalanceStrategy `json:"b"`
	// Listen is the listener, "tcp/30001", "tcp,udp/30001" (diagnostics).
	Listen string `json:"l,omitempty"`
	// Ups are the rendered upstreams, in the manifest's order.
	Ups []docUp `json:"u"`
	// Active is the rotation SetUpstreams chose; nil when every rendered
	// upstream is in rotation with its rendered weight, as after an Apply.
	Active []docUp `json:"a,omitempty"`
}

type docUp struct {
	Addr     string `json:"a"`
	Port     uint32 `json:"p"`
	Weight   uint32 `json:"w"`
	Priority uint32 `json:"pr,omitempty"`
}

func (h *docHop) key() driver.HopKey { return driver.HopKey{RouteID: h.Route, HopIndex: h.Index} }

// rendered answers the hop's rendered upstreams.
func (h *docHop) rendered() []upstream {
	out := make([]upstream, 0, len(h.Ups))
	for _, u := range h.Ups {
		a, err := netip.ParseAddr(u.Addr)
		if err != nil {
			continue
		}
		out = append(out, upstream{addr: a, port: u.Port, weight: u.Weight, priority: u.Priority})
	}
	return out
}

// rotation answers the upstreams in rotation, with their weights.
func (h *docHop) rotation() []driver.Upstream {
	src := h.Active
	if src == nil {
		src = h.Ups
	}
	out := make([]driver.Upstream, len(src))
	for i, u := range src {
		out[i] = driver.Upstream{Address: u.Addr, Port: u.Port, Weight: u.Weight}
	}
	return out
}

// docFor builds the document of an artifact.
func docFor(a driver.Artifact, m *manifest) *hostDoc {
	doc := &hostDoc{V: 1, Node: a.NodeRef, Gen: a.Generation, Hash: a.StateHash, Digest: a.Digest}
	for _, h := range m.hops {
		dh := &docHop{Route: h.key.RouteID, Index: h.key.HopIndex, Balance: h.balance, Listen: h.listenText()}
		for _, u := range h.ups {
			dh.Ups = append(dh.Ups, docUp{Addr: u.addr.String(), Port: u.port, Weight: u.weight, Priority: u.priority})
		}
		doc.Hops = append(doc.Hops, dh)
	}
	return doc
}

func (d *hostDoc) hop(k driver.HopKey) *docHop {
	for _, h := range d.Hops {
		if h.key() == k {
			return h
		}
	}
	return nil
}

// encode answers the document's element comments, in key order.
func (d *hostDoc) encode() []string {
	b, err := json.Marshal(d)
	if err != nil {
		panic(err) // plain structs
	}
	s := base64.RawURLEncoding.EncodeToString(b)
	var out []string
	for len(s) > stateChunk {
		out = append(out, s[:stateChunk])
		s = s[stateChunk:]
	}
	return append(out, s)
}

// decodeDoc reads the document back from the state set's element comments.
func decodeDoc(chunks map[uint64]string) (*hostDoc, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	var b strings.Builder
	for i := range uint64(len(chunks)) {
		c, ok := chunks[i]
		if !ok {
			return nil, fmt.Errorf("state chunk %d missing", i)
		}
		b.WriteString(c)
	}
	raw, err := base64.RawURLEncoding.DecodeString(b.String())
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	var doc hostDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	if doc.V != 1 || doc.Digest == "" {
		return nil, fmt.Errorf("state: version %d, digest %q", doc.V, doc.Digest)
	}
	slices.SortFunc(doc.Hops, func(a, b *docHop) int { return a.key().Compare(b.key()) })
	return &doc, nil
}

// writeDoc writes the commands that replace the state document.
func writeDoc(w *writer, doc *hostDoc) {
	tbl := Family + " " + Table
	w.line("flush set %s %s", tbl, stateSet)
	chunks := doc.encode()
	w.open("add element %s %s", tbl, stateSet)
	for i, c := range chunks {
		w.line("%d comment %q,", i, c)
	}
	w.close()
}

// declareStateSets declares the state sets inside a table block.
func declareStateSets(w *writer) {
	for _, name := range []string{stateSet, sealSet} {
		w.open("set %s", name)
		w.line("type mark")
		w.close()
	}
}
