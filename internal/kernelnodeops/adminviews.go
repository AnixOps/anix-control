package kernelnodeops

import (
	"encoding/json"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"google.golang.org/protobuf/proto"
)

// The administrator's agent routes and the agent session RPCs show the
// same things: the RPCs answer them as the routes render them
// (AgentSession.admin_json, GetAgentSessionResponse.connection_json and
// observed_state_json, GetAgentMonitorResponse.monitor_json), so a
// package's native route answers the legacy route's bytes. Both scrub what
// an agent reported, as every result is scrubbed (section 3.7).

// agentOnlineWindow is how recent a WebSocket agent's last message must be
// for the administrator's agent list to show it online.
const agentOnlineWindow = 60 * time.Second

// AgentListEntryJSON renders a WebSocket agent session as
// GET /api/v2/admin/agent/list lists it, at now.
func AgentListEntryJSON(session agentstreams.Session, now time.Time) []byte {
	entry := map[string]any{
		"node_id":      uint(session.Node.ID),
		"last_seen":    session.LastSeen,
		"version":      session.AgentVersion,
		"system":       json.RawMessage(scrubbedJSON(session.System)),
		"capabilities": session.Capabilities,
		"online":       now.Sub(session.LastSeen) < agentOnlineWindow,
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil
	}
	return encoded
}

// SortAgentSessions orders sessions as the session RPCs list them: by node
// kind, node id and transport.
func SortAgentSessions(sessions []agentstreams.Session) {
	sortSessions(sessions)
}

// agentMonitorView is the snapshot GET /api/v2/admin/agent/monitor answers.
type agentMonitorView struct {
	NodeID    uint            `json:"node_id"`
	System    json.RawMessage `json:"system"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// AgentMonitorJSON renders a WebSocket agent's last monitor snapshot as
// GET /api/v2/admin/agent/monitor answers it.
func AgentMonitorJSON(nodeID uint, system map[string]any, updatedAt time.Time) []byte {
	encoded, err := json.Marshal(agentMonitorView{NodeID: nodeID, System: scrubbedJSON(system), UpdatedAt: updatedAt})
	if err != nil {
		return nil
	}
	return encoded
}

// AgentControlConnectionJSON renders a stream session as the
// administrator's Agent Control status shows it ("connection"): the stream
// manager's snapshot, scrubbed.
func AgentControlConnectionJSON(connection any) []byte {
	return scrubbedJSON(connection)
}

// AgentObservedStateJSON renders the last state an agent reported as the
// administrator's Agent Control status shows it ("observed_state"): the
// agent's message and state scrubbed.
func AgentObservedStateJSON(observed *agentv1pb.ObservedState) []byte {
	if observed == nil {
		return nil
	}
	copied := proto.Clone(observed).(*agentv1pb.ObservedState)
	copied.Message = scrubText(copied.GetMessage(), nil)
	copied.StateJson = scrubBytes(copied.GetStateJson(), nil)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil
	}
	return encoded
}

// AgentAckJSON renders a transport's own acknowledgement as the
// administrator's diagnostic task routes show it ("ack"), scrubbed; nil
// without one.
func AgentAckJSON(ack any) []byte {
	if ack == nil {
		return nil
	}
	return scrubbedJSON(ack)
}

// scrubbedJSON encodes value and scrubs it like a result.
func scrubbedJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("null")
	}
	return scrubBytes(encoded, nil)
}
