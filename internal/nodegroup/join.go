package nodegroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RejoinQuietPeriod is how long a node must not have been seen on the
// Agent Control stream before a machine may rejoin it: a node that is
// online is never handed to another machine.
const RejoinQuietPeriod = 5 * time.Minute

// Join request bounds.
const (
	maxPublicAddresses = 8
	maxConnectHost     = 255
	maxHostname        = 255
)

// Why a join was refused, in its audit entry only.
const (
	refusalUnknownToken = "unknown_token"
	refusalDeletedGroup = "group_deleted"
	refusalExpiredToken = "token_expired"
	refusalMaxNodes     = "max_nodes"
)

var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// JoinRequest is AgentEnrollment.Join's request, with the caller's address.
type JoinRequest struct {
	Token  string
	CSRDER []byte
	// MachineFingerprint is the hex SHA-256 of "anixops-join-v1\n" and the
	// machine ID. It only selects the node a machine may rejoin.
	MachineFingerprint string
	Hostname           string
	RequestedName      string
	ConnectHost        string
	// PublicAddresses are the machine's addresses as Control's HTTPS
	// listener saw them (GET /install/ip), IPv4 first.
	PublicAddresses []string
	Arch            string
	OS              string
	AgentVersion    string
	InstanceID      string
	// Reset never rejoins: the join always creates a node.
	Reset bool
	// PeerAddress is the gRPC peer's address, for the audit entries.
	PeerAddress string
}

// Joined is what a join did.
type Joined struct {
	Issued agentpki.Issued
	Node   agentcontrol.AgentNode
	// NodeName is the node's name in Control.
	NodeName  string
	GroupID   uint
	GroupName string
	// Created is false when the machine rejoined its earlier node.
	Created bool
	// Approval is approved or pending.
	Approval string
	// PublicAddress is the address the node joined from (the first public
	// address), and PeerAddress the gRPC peer's.
	PublicAddress string
	PeerAddress   string
}

// Notification is the administrators' notification of a join: a title, a
// text and the event's data. It never carries the token or the machine
// fingerprint.
func (j Joined) Notification() (title, content string, data map[string]any) {
	verb := "已接入"
	if !j.Created {
		verb = "已重新接入"
	}
	title = fmt.Sprintf("节点%s：%s (%s)", verb, j.NodeName, j.Node)
	content = fmt.Sprintf("节点组 [%s] 的节点 %s (%s) %s，来源地址 %s。", j.GroupName, j.NodeName, j.Node, verb, j.addressForPeople())
	if j.Approval == model.NodeApprovalPending {
		title = fmt.Sprintf("节点待审批：%s (%s)", j.NodeName, j.Node)
		content += "该节点组要求审批：批准前 Control 不会向它下发配置和用户。"
	}
	data = map[string]any{
		"node": j.Node.String(), "node_name": j.NodeName, "group": j.GroupName, "group_id": j.GroupID,
		"created": j.Created, "approval": j.Approval, "public_address": j.PublicAddress,
	}
	return title, content, data
}

func (j Joined) addressForPeople() string {
	switch {
	case j.PublicAddress != "":
		return j.PublicAddress
	case j.PeerAddress != "":
		return j.PeerAddress
	}
	return "未知"
}

// joinRefusal is a refused join: the reason goes to the audit entry, the
// caller gets ErrJoinRejected.
type joinRefusal struct {
	reason string
	group  *model.NodeGroup
}

func (r *joinRefusal) Error() string { return ErrJoinRejected.Error() }
func (r *joinRefusal) Unwrap() error { return ErrJoinRejected }

// Join makes a machine a node of the group whose token it presents, in one
// transaction (docs/design/node-onboarding, "Join Protocol"):
//
//  1. It finds the group by the token's hash and refuses (ErrJoinRejected,
//     audited with the reason and the peer's address) an unknown token, a
//     deleted group or an expired token.
//  2. A machine that lost its local state rejoins its earlier node when the
//     rejoin rule holds (rejoinTarget): its certificates are revoked and a
//     new one is issued for it.
//  3. Otherwise it creates a node, refusing a group at its node cap: the
//     requested name, else the host name, made unique in the group with a
//     -2, -3 suffix; the connect host, else the first public address; the
//     group's defaults; pending approval when the group requires it. A
//     proxy node gets the template's protocols with their own secrets and
//     ports, in the group's subscription groups; a forward node is created
//     as the forward inventory creates one.
//  4. It records a join_token enrollment, issues the certificate as Enroll
//     does and writes the agent_join audit entry.
//
// After the commit it runs OnJoin in its own goroutine.
func (s *Service) Join(ctx context.Context, request JoinRequest) (Joined, error) {
	if s.PKI == nil {
		return Joined{}, agentpki.ErrDisabled
	}
	request, err := normalizeJoin(request)
	if err != nil {
		return Joined{}, err
	}
	var (
		joined      Joined
		afterCommit []func()
	)
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		joined, afterCommit = Joined{}, nil
		group, err := s.joinGroup(tx, request.Token)
		if err != nil {
			return err
		}
		joined.GroupID, joined.GroupName, joined.PeerAddress = group.ID, group.Name, request.PeerAddress
		if len(request.PublicAddresses) > 0 {
			joined.PublicAddress = request.PublicAddresses[0]
		}
		target, err := s.rejoinTarget(tx, group, request)
		if err != nil {
			return err
		}
		if target != nil {
			if err := agentpki.RevokeNode(ctx, tx, target.node, agentpki.RevokeReasonRejoined); err != nil {
				return err
			}
			afterCommit = append(afterCommit, agentpki.ForgetRevocations)
			joined.Node, joined.NodeName, joined.Approval = target.node, target.name, approvalOf(target.approval)
		} else {
			if group.MaxNodes > 0 {
				var live int64
				if err := liveNodes(tx, group).Count(&live).Error; err != nil {
					return err
				}
				if live >= int64(group.MaxNodes) {
					return &joinRefusal{reason: refusalMaxNodes, group: &group}
				}
			}
			created, after, err := s.createNode(tx, group, request)
			if err != nil {
				return err
			}
			afterCommit = append(afterCommit, after...)
			joined.Node, joined.NodeName, joined.Approval, joined.Created = created.node, created.name, created.approval, true
		}
		issued, err := s.PKI.IssueJoinTx(ctx, tx, agentpki.JoinIssue{
			Node: joined.Node, CSRDER: request.CSRDER, AgentVersion: request.AgentVersion, InstanceID: request.InstanceID,
			RemoteAddr: request.PeerAddress,
			Audit: map[string]any{
				"group": group.Name, "group_id": group.ID, "node_name": joined.NodeName, "created": joined.Created,
				"approval": joined.Approval, "public_address": joined.PublicAddress, "hostname": request.Hostname,
				"arch": request.Arch, "os": request.OS,
			},
		})
		if err != nil {
			return err
		}
		joined.Issued = issued
		return nil
	})
	if err != nil {
		var refusal *joinRefusal
		if errors.As(err, &refusal) {
			s.auditRefusal(ctx, refusal, request)
			return Joined{}, ErrJoinRejected
		}
		return Joined{}, err
	}
	for _, after := range afterCommit {
		after()
	}
	if s.OnJoin != nil {
		go s.OnJoin(joined)
	}
	return joined, nil
}

func approvalOf(approval string) string {
	if approval == model.NodeApprovalPending {
		return model.NodeApprovalPending
	}
	return model.NodeApprovalApproved
}

// normalizeJoin checks a join request's fields and returns them trimmed.
func normalizeJoin(request JoinRequest) (JoinRequest, error) {
	request.Token = strings.TrimSpace(request.Token)
	request.PeerAddress = strings.TrimSpace(request.PeerAddress)
	if len(request.CSRDER) == 0 {
		return request, fmt.Errorf("%w: csr_der is required", ErrInvalidJoin)
	}
	request.MachineFingerprint = strings.ToLower(strings.TrimSpace(request.MachineFingerprint))
	if request.MachineFingerprint != "" && !fingerprintPattern.MatchString(request.MachineFingerprint) {
		return request, fmt.Errorf("%w: machine_fingerprint must be a hex SHA-256", ErrInvalidJoin)
	}
	request.RequestedName = strings.TrimSpace(request.RequestedName)
	if request.RequestedName != "" && (hasControl(request.RequestedName) || utf8.RuneCountInString(request.RequestedName) > maxNodeNameRunes) {
		return request, fmt.Errorf("%w: requested_name must be at most %d characters without control characters", ErrInvalidJoin, maxNodeNameRunes)
	}
	request.Hostname = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(request.Hostname))
	if len(request.Hostname) > maxHostname {
		request.Hostname = request.Hostname[:maxHostname]
	}
	request.ConnectHost = strings.TrimSpace(request.ConnectHost)
	if request.ConnectHost != "" && (len(request.ConnectHost) > maxConnectHost || hasControl(request.ConnectHost) ||
		strings.ContainsAny(request.ConnectHost, " /\\?#@")) {
		return request, fmt.Errorf("%w: connect_host must be a host name or an IP address", ErrInvalidJoin)
	}
	if len(request.PublicAddresses) > maxPublicAddresses {
		return request, fmt.Errorf("%w: at most %d public_addresses", ErrInvalidJoin, maxPublicAddresses)
	}
	addresses := make([]string, 0, len(request.PublicAddresses))
	seen := map[string]bool{}
	for _, raw := range request.PublicAddresses {
		address, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil || address.Zone() != "" {
			return request, fmt.Errorf("%w: public_addresses must be IP addresses", ErrInvalidJoin)
		}
		text := address.Unmap().String()
		if !seen[text] {
			seen[text] = true
			addresses = append(addresses, text)
		}
	}
	request.PublicAddresses = addresses
	request.Arch, request.OS = truncate(strings.TrimSpace(request.Arch), 32), truncate(strings.TrimSpace(request.OS), 32)
	return request, nil
}

// joinGroup finds and locks the group of token, or refuses it. The lock
// orders concurrent joins of a group: each sees the nodes the others
// created, for the node cap and the names.
func (s *Service) joinGroup(tx *gorm.DB, token string) (model.NodeGroup, error) {
	if !ValidToken(token) {
		return model.NodeGroup{}, &joinRefusal{reason: refusalUnknownToken}
	}
	var groups []model.NodeGroup
	if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("join_token_hash = ?", HashToken(token)).Limit(1).Find(&groups).Error; err != nil {
		return model.NodeGroup{}, err
	}
	if len(groups) == 0 {
		return model.NodeGroup{}, &joinRefusal{reason: refusalUnknownToken}
	}
	group := groups[0]
	switch {
	case group.Deleted != 0:
		return model.NodeGroup{}, &joinRefusal{reason: refusalDeletedGroup, group: &group}
	case group.TokenExpiresAt != nil && !group.TokenExpiresAt.After(s.now()):
		return model.NodeGroup{}, &joinRefusal{reason: refusalExpiredToken, group: &group}
	}
	return group, nil
}

// auditRefusal records a refused join, outside the join's transaction,
// with the peer's address and the reason. It never fails the answer.
func (s *Service) auditRefusal(ctx context.Context, refusal *joinRefusal, request JoinRequest) {
	content := map[string]any{"reason": refusal.reason, "peer_address": request.PeerAddress, "hostname": request.Hostname}
	if len(request.PublicAddresses) > 0 {
		content["public_address"] = request.PublicAddresses[0]
	}
	var groupID uint
	if refusal.group != nil {
		groupID = refusal.group.ID
		content["group"], content["group_id"] = refusal.group.Name, refusal.group.ID
	}
	encoded, err := json.Marshal(content)
	if err == nil {
		var target *uint
		if groupID != 0 {
			target = &groupID
		}
		err = s.DB.WithContext(context.WithoutCancel(ctx)).Create(&model.OperationLog{
			Username: auditJoinRejectedActorUnknown, Action: AuditActionJoinRejected, Module: auditModule,
			TargetType: auditTargetGroup, TargetID: target, Content: string(encoded), IP: truncate(request.PeerAddress, 45), Status: 2,
		}).Error
	}
	if err != nil {
		slog.Warn("node group: a refused join could not be audited", "component", "node-group", "reason", refusal.reason,
			"peer", request.PeerAddress, "error", err)
	}
}

// joinedNode is the node a join created or rejoins.
type joinedNode struct {
	node     agentcontrol.AgentNode
	name     string
	approval string
}

// rejoinTarget returns the node a machine rejoins, or nil. All of these must
// hold: reset is false; a live, enabled node of the same group has the
// machine's fingerprint; the public address it joined from is one of the
// request's public addresses; and it has not been seen on the Agent Control
// stream (nor by a heartbeat) for RejoinQuietPeriod. Cloned images share
// /etc/machine-id, so the fingerprint alone never hands a machine another
// machine's node. Of several candidates the newest node wins.
func (s *Service) rejoinTarget(tx *gorm.DB, group model.NodeGroup, request JoinRequest) (*joinedNode, error) {
	if request.Reset || request.MachineFingerprint == "" || len(request.PublicAddresses) == 0 {
		return nil, nil
	}
	cutoff := s.now().Add(-RejoinQuietPeriod)
	type candidate struct {
		node      agentcontrol.AgentNode
		name      string
		approval  string
		address   string
		lastCheck *time.Time
	}
	var candidates []candidate
	if group.Kind == model.NodeGroupKindForward {
		var nodes []model.ForwardNode
		if err := liveNodes(tx, group).Where("machine_fingerprint = ? AND enabled = ?", request.MachineFingerprint, true).
			Order("id DESC").Find(&nodes).Error; err != nil {
			return nil, err
		}
		for _, node := range nodes {
			lastCheck := node.LastCheck
			candidates = append(candidates, candidate{
				node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(node.ID)}, // #nosec G115 -- node ids are below 2^32.
				name: node.Name, approval: node.Approval, address: node.LastPublicAddress, lastCheck: &lastCheck,
			})
		}
	} else {
		var nodes []model.Node
		if err := liveNodes(tx, group).Where("machine_fingerprint = ? AND status <> ?", request.MachineFingerprint, model.NodeStatusDisabled).
			Order("id DESC").Find(&nodes).Error; err != nil {
			return nil, err
		}
		for _, node := range nodes {
			var lastCheck *time.Time
			if node.LastCheckAt != nil {
				at := time.Unix(*node.LastCheckAt, 0).UTC()
				lastCheck = &at
			}
			candidates = append(candidates, candidate{
				node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(node.ID)}, // #nosec G115 -- node ids are below 2^32.
				name: node.Name, approval: node.Approval, address: node.LastPublicAddress, lastCheck: lastCheck,
			})
		}
	}
	for _, candidate := range candidates {
		if candidate.address == "" || !contains(request.PublicAddresses, candidate.address) {
			continue
		}
		if candidate.lastCheck != nil && candidate.lastCheck.After(cutoff) {
			continue
		}
		var seen int64
		if err := tx.Model(&model.AgentTransport{}).
			Where("node_kind = ? AND node_id = ? AND transport IN ? AND last_seen_at > ?", candidate.node.Kind, candidate.node.ID,
				[]string{model.AgentTransportMTLSStream, model.AgentTransportAPIKeyStream}, cutoff).
			Count(&seen).Error; err != nil {
			return nil, err
		}
		if seen > 0 {
			continue
		}
		return &joinedNode{node: candidate.node, name: candidate.name, approval: candidate.approval}, nil
	}
	return nil, nil
}

// createNode creates the node of a join. It returns what to run after the
// commit.
func (s *Service) createNode(tx *gorm.DB, group model.NodeGroup, request JoinRequest) (joinedNode, []func(), error) {
	name, err := uniqueName(tx, group, nodeName(request))
	if err != nil {
		return joinedNode{}, nil, err
	}
	host := request.ConnectHost
	if host == "" && len(request.PublicAddresses) > 0 {
		host = request.PublicAddresses[0]
	}
	if host == "" {
		// Without an address from Control's HTTPS listener (an offline
		// install that could not reach it), the gRPC peer's address is the
		// best guess; behind a proxy it is the proxy's, and the
		// administrator corrects it.
		host = request.PeerAddress
	}
	if host == "" {
		return joinedNode{}, nil, fmt.Errorf("%w: connect_host or public_addresses is required", ErrInvalidJoin)
	}
	approval := model.NodeApprovalApproved
	if group.RequireApproval {
		approval = model.NodeApprovalPending
	}
	var publicAddress string
	if len(request.PublicAddresses) > 0 {
		publicAddress = request.PublicAddresses[0]
	}
	groupID := group.ID
	if group.Kind == model.NodeGroupKindForward {
		created, after, err := s.forward().CreateForwardNodeTx(tx, &forwardv1.ForwardNodeRecord{
			Name: name, Host: host, Enabled: true, Transport: forwardv1.NodeTransport_NODE_TRANSPORT_AGENT,
		}, func(node *model.ForwardNode) {
			node.NodeGroupID, node.MachineFingerprint, node.Approval, node.LastPublicAddress = &groupID, request.MachineFingerprint, approval, publicAddress
			// The group's tags, but never the inventory's mark of an
			// Ansible machine: a joined node runs the Agent.
			var tags []string
			for _, tag := range tagsOf(group) {
				if tag != service.ForwardNodeInventoryTagAnsibleMachine {
					tags = append(tags, tag)
				}
			}
			if len(tags) > 0 {
				encoded, _ := json.Marshal(tags)
				node.Tags = string(encoded)
			}
		})
		if err != nil {
			return joinedNode{}, nil, err
		}
		return joinedNode{
			node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: uint32(created.ID)}, // #nosec G115 -- node ids are below 2^32.
			name: created.Name, approval: approval,
		}, []func(){after}, nil
	}

	node := model.Node{
		Name: name, Host: host, Port: 443, Status: model.NodeStatusPending, Rate: group.Rate, TrafficRate: 1, Sort: group.Sort, Show: 1,
		MonthlyResetDay: group.ResetDay, NodeGroupID: &groupID, MachineFingerprint: request.MachineFingerprint,
		Approval: approval, LastPublicAddress: publicAddress, RuntimeHealthy: true,
	}
	if group.Tags != "" {
		tags := group.Tags
		node.Tags = &tags
	}
	if group.MonthlyLimitBytes > 0 {
		limit := group.MonthlyLimitBytes
		node.MonthlyLimit = &limit
	}
	if request.AgentVersion != "" {
		version := truncate(request.AgentVersion, 50)
		node.ServerVersion = &version
	}
	if request.OS != "" || request.Arch != "" {
		system := strings.Trim(request.OS+"/"+request.Arch, "/")
		node.ServerOS = &system
	}
	// The node's API key and shared secret, as every proxy node has them
	// until node credentials are removed; nobody is shown them.
	credentials, err := service.GenerateProxyNodeCredentials(true, true)
	if err != nil {
		return joinedNode{}, nil, err
	}
	credentials.Apply(&node)
	if err := tx.Omit(clause.Associations).Create(&node).Error; err != nil {
		return joinedNode{}, nil, err
	}
	if err := nodesecrets.Sync(tx, nodesecrets.TableNode, node.ID); err != nil {
		return joinedNode{}, nil, err
	}
	if err := createProtocols(tx, group, node.ID); err != nil {
		return joinedNode{}, nil, err
	}
	nodeID := node.ID
	return joinedNode{
		node: agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(node.ID)}, // #nosec G115 -- node ids are below 2^32.
		name: node.Name, approval: approval,
	}, []func(){func() { service.DropNodeCache(nodeID) }}, nil
}

// createProtocols creates the group template's protocols on a new proxy
// node, each with its own secrets and port, in the group's subscription
// groups (those that still exist).
func createProtocols(tx *gorm.DB, group model.NodeGroup, nodeID uint) error {
	entries := templateOf(group)
	if len(entries) == 0 {
		return nil
	}
	var subscriptionGroups []model.SubscriptionGroup
	if ids := subscriptionGroupsOf(group); len(ids) > 0 {
		if err := tx.Where("id IN ?", ids).Order("id").Find(&subscriptionGroups).Error; err != nil {
			return err
		}
	}
	ports, err := assignPorts(entries)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJoinTemplate, err)
	}
	for i, entry := range entries {
		protocol, err := buildProtocol(entry, i, nodeID, ports[i])
		if err != nil {
			return err
		}
		if err := service.ValidateNodeProtocol(&protocol); err != nil {
			return fmt.Errorf("%w: protocol %q: %v", ErrJoinTemplate, entry.Name, err)
		}
		if err := tx.Omit(clause.Associations).Create(&protocol).Error; err != nil {
			return err
		}
		if err := nodesecrets.Sync(tx, nodesecrets.TableNodeProtocol, protocol.ID); err != nil {
			return err
		}
		if len(subscriptionGroups) > 0 {
			if err := tx.Model(&protocol).Omit("SubscriptionGroups.*").Association("SubscriptionGroups").Append(subscriptionGroups); err != nil {
				return err
			}
		}
	}
	return nil
}

// nodeName is the name a join asks for: the requested name, else the host
// name, else "node", at most maxNodeNameRunes characters.
func nodeName(request JoinRequest) string {
	name := request.RequestedName
	if name == "" {
		name = request.Hostname
	}
	if name == "" {
		name = "node"
	}
	if utf8.RuneCountInString(name) > maxNodeNameRunes {
		name = string([]rune(name)[:maxNodeNameRunes])
	}
	return name
}

// uniqueName returns base, or base-2, base-3, ... : the first name no live
// node of the group has.
func uniqueName(tx *gorm.DB, group model.NodeGroup, base string) (string, error) {
	var names []string
	if err := liveNodes(tx, group).Pluck("name", &names).Error; err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(names))
	for _, name := range names {
		taken[name] = true
	}
	if !taken[base] {
		return base, nil
	}
	for suffix := 2; ; suffix++ {
		if candidate := base + "-" + strconv.Itoa(suffix); !taken[candidate] {
			return candidate, nil
		}
	}
}
