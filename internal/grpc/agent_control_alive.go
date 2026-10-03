package grpc

import (
	"context"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

// The alive list on the Agent Control stream (alive.v1; PROTOCOL.md,
// "Alive list"). UniProxy alivelist answers every user's online device
// count across all nodes, and agents enforce NodeUser.device_limit against
// it; an agent on users.v1 alone saw only its own node's connections. When
// an agent's Hello lists alive.v1, the HelloAck advertises it back (proxy
// nodes, which serve users) and Control sends the same counts as an
// AliveList: after HelloAck, then whenever they change, at most once per
// aliveListInterval, the interval at which agents pull alivelist by
// default (pull_interval 60 s).
//
// The counts come from service.UserAliveCounts, the source of the HTTP
// handler, read at most once per aliveListRefresh for the whole process and
// shared by every session.

// Bounds of the alive list. Variables so tests can lower them.
var (
	// aliveListInterval is how often a session's list is checked for a
	// change and sent again.
	aliveListInterval = time.Minute
	// aliveListRefresh is how long the counts are reused across sessions.
	aliveListRefresh = 10 * time.Second
	// aliveListPageSize is the most users in one AliveList message (about
	// 10 bytes each on the wire).
	aliveListPageSize = 10000
	// aliveCountSource reads the counts; service.UserAliveCounts, as the
	// HTTP alivelist.
	aliveCountSource = service.UserAliveCounts
)

// servesAlive tells whether the HelloAck advertises alive.v1: when the
// agent lists it and the stream's node is a proxy node, whose users have
// device limits.
func (s *AgentControlGRPCServer) servesAlive(node agentcontrol.AgentNode, agent []*agentv1pb.Capability) bool {
	return node.Kind == agentcontrol.NodeKindProxy &&
		agentcontrol.HasCapabilityVersion(agent, agentcontrol.CapabilityAlive, agentcontrol.CapabilityVersionV1)
}

// aliveSnapshot is the counts at one time, in user id order. generation
// changes only when the counts do.
type aliveSnapshot struct {
	generation uint64
	computedAt time.Time
	entries    []*agentv1pb.UserAlive
}

// aliveCounts shares one read of the counts between sessions.
type aliveCounts struct {
	mu       sync.Mutex
	snapshot aliveSnapshot
	readAt   time.Time
}

var sharedAliveCounts = &aliveCounts{}

// current returns the counts, read again when the last read is older than
// aliveListRefresh. A failed read keeps the last counts; ok is false when
// there are none yet.
func (c *aliveCounts) current(now time.Time) (aliveSnapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.readAt.IsZero() && now.Sub(c.readAt) < aliveListRefresh {
		return c.snapshot, true
	}
	counts, err := aliveCountSource()
	if err != nil {
		slog.Warn("agent alive list: the online counts could not be read", "component", "agent-control", "error", err)
		return c.snapshot, c.snapshot.generation != 0
	}
	entries := make([]*agentv1pb.UserAlive, 0, len(counts))
	for userID, count := range counts {
		if userID == 0 || count <= 0 {
			continue
		}
		entries = append(entries, &agentv1pb.UserAlive{UserId: userID, AliveCount: uint32(min(count, math.MaxUint32))})
	}
	slices.SortFunc(entries, func(a, b *agentv1pb.UserAlive) int {
		switch {
		case a.UserId < b.UserId:
			return -1
		case a.UserId > b.UserId:
			return 1
		}
		return 0
	})
	c.readAt = now
	if c.snapshot.generation == 0 || !sameAliveEntries(c.snapshot.entries, entries) {
		c.snapshot = aliveSnapshot{generation: c.snapshot.generation + 1, computedAt: now, entries: entries}
	}
	return c.snapshot, true
}

func sameAliveEntries(a, b []*agentv1pb.UserAlive) bool {
	return slices.EqualFunc(a, b, func(x, y *agentv1pb.UserAlive) bool {
		return x.UserId == y.UserId && x.AliveCount == y.AliveCount
	})
}

// startAliveList serves a session's alive lists in the background. The
// returned function stops the sender and waits for it.
func (s *AgentControlGRPCServer) startAliveList(ctx context.Context, connection *AgentControlConnection) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveAliveList(ctx, connection, sharedAliveCounts)
	}()
	return func() {
		cancel()
		<-done
	}
}

// serveAliveList sends the session's first list at once, then a list
// whenever the counts changed, checked every aliveListInterval, until ctx
// ends or a send fails (the stream is gone).
func serveAliveList(ctx context.Context, connection *AgentControlConnection, counts *aliveCounts) {
	var revision, sent uint64
	ticker := time.NewTicker(aliveListInterval)
	defer ticker.Stop()
	for {
		if snapshot, ok := counts.current(time.Now()); ok && snapshot.generation != sent {
			revision++
			if err := sendAliveList(connection, revision, snapshot); err != nil {
				return
			}
			sent = snapshot.generation
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sendAliveList sends one list in pages of at most aliveListPageSize
// users, each with the same revision; the last has last_page. An empty
// list is one empty page.
func sendAliveList(connection *AgentControlConnection, revision uint64, snapshot aliveSnapshot) error {
	entries := snapshot.entries
	for {
		page := entries[:min(len(entries), aliveListPageSize)]
		entries = entries[len(page):]
		if err := connection.send(&agentv1pb.ControlToAgent{
			RequestId:    newAgentControlID("alive"),
			NodeId:       connection.NodeID,
			SentAtUnixMs: time.Now().UnixMilli(),
			Payload: &agentv1pb.ControlToAgent_AliveList{AliveList: &agentv1pb.AliveList{
				Revision: revision, LastPage: len(entries) == 0, Entries: page,
				ComputedAtUnixMs: snapshot.computedAt.UnixMilli(),
			}},
		}); err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
	}
}
