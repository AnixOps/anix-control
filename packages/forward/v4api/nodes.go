package v4api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/protobuf/proto"
)

// The node inventory and the forward node registry. These rewrite the v2
// forward node and Ansible machine routes on ForwardControl:
//   - list, create, get, update, delete and toggle map one to one;
//   - check (Control dialled the node) is the node view: the Agent's latest
//     report says whether the node applied its desired generation, its hop
//     errors and its upstream health;
//   - sync-stats (Control pulled gost's metrics) is the traffic ledger the
//     nodes' reports fill (GET /stats?node_ref=).
//
// An Ansible machine is a forward node with transport
// NODE_TRANSPORT_ANSIBLE; /ansible-machines is that part of /nodes, by
// numeric id.

func nodeTransport(raw string) (forwardv1.NodeTransport, bool) {
	switch strings.ToLower(raw) {
	case "":
		return forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED, true
	case "agent", "node_transport_agent":
		return forwardv1.NodeTransport_NODE_TRANSPORT_AGENT, true
	case "ansible", "node_transport_ansible":
		return forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE, true
	}
	return 0, false
}

func (s *Service) listNodes(ctx context.Context, request Request, _ map[string]string) Response {
	transport, ok := nodeTransport(query(request, "transport"))
	if !ok {
		return failure(http.StatusBadRequest, "invalid_request", "transport must be agent or ansible", nil)
	}
	return s.nodeList(ctx, request, query(request, "kind"), transport)
}

// nodeList answers the nodes with can_delete: whether the caller may
// DELETE one (a super administrator, as the kernel says; F5b D7).
func (s *Service) nodeList(ctx context.Context, request Request, kind string, transport forwardv1.NodeTransport) Response {
	answer, err := s.Forward.ListNodes(ctx, &forwardv1.ListNodesRequest{Kind: kind, Transport: transport})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"nodes": pjList(answer.GetNodes()), "can_delete": request.SuperAdmin})
}

// createNode: the body is {"node": ForwardNodeRecord, "settings":
// NodeSettings}; settings are optional.
func (s *Service) createNode(ctx context.Context, request Request, _ map[string]string) Response {
	return s.create(ctx, request, forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED)
}

func (s *Service) create(ctx context.Context, request Request, forced forwardv1.NodeTransport) Response {
	body := &forwardv1.CreateForwardNodeRequest{}
	if answer := decode(request.Body, body, false); answer != nil {
		return *answer
	}
	if body.GetNode() == nil {
		return failure(http.StatusBadRequest, "invalid_request", "node is required", nil)
	}
	if forced != forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED {
		body.Node.Transport = forced
	}
	body.RequestId = s.requestID(request, "")
	answer, err := s.Forward.CreateForwardNode(ctx, body)
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusCreated, map[string]any{"node": pj(answer.GetNode())})
}

// forwardID parses a forward node reference ("forward-<id>").
func forwardID(ref string) (uint64, bool) {
	raw, ok := strings.CutPrefix(ref, "forward-")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseUint(raw, 10, 32)
	return id, err == nil && id > 0
}

// getNode answers GetNodeResponse: the node, its desired state and its
// latest report (applied generation, hop errors, upstream health).
func (s *Service) getNode(ctx context.Context, _ Request, params map[string]string) Response {
	answer, err := s.Forward.GetNode(ctx, &forwardv1.GetNodeRequest{NodeRef: params["ref"]})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, nodeView(answer))
}

func nodeView(answer *forwardv1.GetNodeResponse) map[string]any {
	view := map[string]any{"node": pj(answer.GetNode()), "state": nil, "report": nil}
	if answer.GetState() != nil {
		view["state"] = pj(answer.GetState())
	}
	if answer.GetReport() != nil {
		view["report"] = pj(answer.GetReport())
	}
	return view
}

// updateNode: the body is the node's whole ForwardNodeRecord; the path
// names it. Only forward nodes have one: a proxy node's row is the
// proxy-node package's, and its forwarding settings are PUT .../settings.
func (s *Service) updateNode(ctx context.Context, request Request, params map[string]string) Response {
	return s.update(ctx, request, params["ref"], forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED)
}

func (s *Service) update(ctx context.Context, request Request, ref string, forced forwardv1.NodeTransport) Response {
	id, ok := forwardID(ref)
	if !ok {
		return failure(http.StatusBadRequest, "invalid_request", "only a forward node (forward-<id>) has editable fields", nil)
	}
	record := &forwardv1.ForwardNodeRecord{}
	if answer := decode(request.Body, record, false); answer != nil {
		return *answer
	}
	if record.GetId() != 0 && record.GetId() != id {
		return failure(http.StatusBadRequest, "invalid_request", "the body's id is not the path's", nil)
	}
	record.Id = id
	if forced != forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED {
		if answer := s.requireTransport(ctx, ref, forced); answer != nil {
			return *answer
		}
		record.Transport = forced
	}
	answer, err := s.Forward.UpdateForwardNode(ctx, &forwardv1.UpdateForwardNodeRequest{RequestId: s.requestID(request, ""), Node: record})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"node": pj(answer.GetNode()), "violations": violationsOf(answer.GetViolations())})
}

func (s *Service) deleteNode(ctx context.Context, request Request, params map[string]string) Response {
	return s.remove(ctx, request, params["ref"], forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED)
}

func (s *Service) remove(ctx context.Context, request Request, ref string, forced forwardv1.NodeTransport) Response {
	id, ok := forwardID(ref)
	if !ok {
		return failure(http.StatusBadRequest, "invalid_request", "only a forward node (forward-<id>) can be deleted here", nil)
	}
	if forced != forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED {
		if answer := s.requireTransport(ctx, ref, forced); answer != nil {
			return *answer
		}
	}
	if _, err := s.Forward.DeleteForwardNode(ctx, &forwardv1.DeleteForwardNodeRequest{RequestId: s.requestID(request, ""), Id: id}); err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"deleted": ref})
}

// setNodeSettings: the body is NodeSettings; any proxy or forward node.
// A replan the settings make impossible still stores them: the answer's
// violations say why every node keeps its generation.
func (s *Service) setNodeSettings(ctx context.Context, request Request, params map[string]string) Response {
	settings := &forwardv1.NodeSettings{}
	if answer := decode(request.Body, settings, true); answer != nil {
		return *answer
	}
	answer, err := s.Forward.SetNodeSettings(ctx, &forwardv1.SetNodeSettingsRequest{NodeRef: params["ref"], Settings: settings})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"node": pj(answer.GetNode()), "violations": violationsOf(answer.GetViolations())})
}

func (s *Service) toggleNode(ctx context.Context, request Request, params map[string]string) Response {
	return s.toggle(ctx, request, params["ref"], forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED)
}

// toggle: the body is {"enabled": bool}. Disabling a node a route uses is
// refused (409 refused, code node_in_use).
func (s *Service) toggle(ctx context.Context, request Request, ref string, forced forwardv1.NodeTransport) Response {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(request.Body, &body); err != nil || body.Enabled == nil {
		return failure(http.StatusBadRequest, "invalid_request", `the body must be {"enabled": true|false}`, nil)
	}
	id, ok := forwardID(ref)
	if !ok {
		return failure(http.StatusBadRequest, "invalid_request", "only a forward node (forward-<id>) can be toggled", nil)
	}
	current, err := s.Forward.GetNode(ctx, &forwardv1.GetNodeRequest{NodeRef: ref})
	if err != nil {
		return fromStatus(err)
	}
	record := current.GetNode().GetRecord()
	if record == nil || (forced != forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED && record.GetTransport() != forced) {
		return failure(http.StatusNotFound, "not_found", "forward node not found", nil)
	}
	record = proto.Clone(record).(*forwardv1.ForwardNodeRecord)
	record.Id, record.Enabled = id, *body.Enabled
	suffix := "disable"
	if *body.Enabled {
		suffix = "enable"
	}
	answer, err := s.Forward.UpdateForwardNode(ctx, &forwardv1.UpdateForwardNodeRequest{RequestId: s.requestID(request, suffix), Node: record})
	if err != nil {
		return fromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"node": pj(answer.GetNode()), "violations": violationsOf(answer.GetViolations())})
}

// requireTransport answers 404 unless ref is a forward node reached by
// transport, so /ansible-machines never reaches an Agent node.
func (s *Service) requireTransport(ctx context.Context, ref string, transport forwardv1.NodeTransport) *Response {
	current, err := s.Forward.GetNode(ctx, &forwardv1.GetNodeRequest{NodeRef: ref})
	if err != nil {
		answer := fromStatus(err)
		return &answer
	}
	if current.GetNode().GetRecord().GetTransport() != transport {
		answer := failure(http.StatusNotFound, "not_found", "forward node not found", nil)
		return &answer
	}
	return nil
}

// ansibleRef is the reference of /ansible-machines/{id}.
func ansibleRef(params map[string]string) (string, *Response) {
	id, err := strconv.ParseUint(params["id"], 10, 32)
	if err != nil || id == 0 {
		answer := failure(http.StatusBadRequest, "invalid_request", "id must be a forward node id", nil)
		return "", &answer
	}
	return "forward-" + strconv.FormatUint(id, 10), nil
}

const ansible = forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE

func (s *Service) listAnsible(ctx context.Context, request Request, _ map[string]string) Response {
	return s.nodeList(ctx, request, "forward", ansible)
}

func (s *Service) createAnsible(ctx context.Context, request Request, _ map[string]string) Response {
	return s.create(ctx, request, ansible)
}

func (s *Service) getAnsible(ctx context.Context, _ Request, params map[string]string) Response {
	ref, bad := ansibleRef(params)
	if bad != nil {
		return *bad
	}
	answer, err := s.Forward.GetNode(ctx, &forwardv1.GetNodeRequest{NodeRef: ref})
	if err != nil {
		return fromStatus(err)
	}
	if answer.GetNode().GetRecord().GetTransport() != ansible {
		return failure(http.StatusNotFound, "not_found", "forward node not found", nil)
	}
	return data(http.StatusOK, nodeView(answer))
}

func (s *Service) updateAnsible(ctx context.Context, request Request, params map[string]string) Response {
	ref, bad := ansibleRef(params)
	if bad != nil {
		return *bad
	}
	return s.update(ctx, request, ref, ansible)
}

func (s *Service) deleteAnsible(ctx context.Context, request Request, params map[string]string) Response {
	ref, bad := ansibleRef(params)
	if bad != nil {
		return *bad
	}
	return s.remove(ctx, request, ref, ansible)
}

func (s *Service) toggleAnsible(ctx context.Context, request Request, params map[string]string) Response {
	ref, bad := ansibleRef(params)
	if bad != nil {
		return *bad
	}
	return s.toggle(ctx, request, ref, ansible)
}
