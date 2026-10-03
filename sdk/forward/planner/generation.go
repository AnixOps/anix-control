package planner

import (
	"crypto/sha256"
	"encoding/hex"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/protobuf/proto"
)

// Generation is what Control keeps per node between plans: the generation
// it last handed out and that state's hash.
type Generation struct {
	Generation uint64
	StateHash  string
}

// StateHash answers the lowercase hex SHA-256 of the deterministic
// protobuf encoding of a NodeForwardState holding only hops
// (forward-sdk.md section 5.4). The planner sorts hops by route and hop, so
// the same hops always hash the same.
func StateHash(hops []*forwardv1.NodeHop) string {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(&forwardv1.NodeForwardState{Hops: hops})
	if err != nil {
		// A NodeHop has no required fields and no invalid UTF-8 the planner
		// writes, so encoding cannot fail.
		panic("planner: encoding hops: " + err.Error())
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Stamp sets every state's state_hash and generation and answers the
// generations to keep for the next plan. A node keeps its previous
// generation while its hash is unchanged; otherwise the generation is the
// previous one plus one (1 for a node without one). A change to one route
// therefore bumps only the nodes it touches. Previous generations of nodes
// that have no state are carried over unchanged.
func Stamp(states map[string]*forwardv1.NodeForwardState, previous map[string]Generation) map[string]Generation {
	out := make(map[string]Generation, len(previous)+len(states))
	for ref, g := range previous {
		out[ref] = g
	}
	for ref, s := range states {
		hash := StateHash(s.GetHops())
		g := previous[ref]
		if g.StateHash != hash || g.Generation == 0 {
			g = Generation{Generation: g.Generation + 1, StateHash: hash}
		}
		s.Generation, s.StateHash = g.Generation, g.StateHash
		out[ref] = g
	}
	return out
}
