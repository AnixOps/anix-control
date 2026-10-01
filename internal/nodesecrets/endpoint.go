package nodesecrets

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strconv"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Endpoint pinning of forward node tokens (node-ops-service.md section 3.8,
// decision D12). An address is a credential: whoever can write a forward
// node's host or api_port could point the kernel's next NodeX or gost call,
// which carries the node's token, at an address of their own. So the token
// is pinned to the endpoint it was bound to, host:api_port as recorded in
// v4_kernel_node_credential.endpoint by the kernel's own forward node
// writers (Sync), and the kernel presents it only there.

// ErrEndpointUnconfirmed is a forward node whose address no longer matches
// the endpoint its token is pinned to: the kernel does not present the
// token there until an administrator's own forward node update confirms the
// address (ENDPOINT_UNCONFIRMED). Its text names no address and no value.
var ErrEndpointUnconfirmed = errors.New("the forward node's address is not the one its token is pinned to; an administrator's forward node update confirms it")

// Pin reasons, for the counter.
const (
	// PinUnconfirmed: the row's address differs from the pinned endpoint.
	PinUnconfirmed = "unconfirmed"
	// PinUnpinned: the token has no pin yet (no credential row; the
	// backfill has not run), so it was presented as before the split.
	PinUnpinned = "unpinned"
)

// ForwardNodeEndpoint is the address a forward node's token is pinned to:
// its management API, host:api_port. A node without a host or an API port
// has no endpoint, so its token is pinned to nothing and never presented.
func ForwardNodeEndpoint(node *model.ForwardNode) string {
	if node == nil {
		return ""
	}
	return forwardNodeEndpoint(node.Host, node.APIPort)
}

// ForwardNodeTokenAt answers the API token of the forward node node for a
// call to the node's management API at the node's current address, or
// ErrEndpointUnconfirmed when the kernel must not present it there.
//
// The rule: the token is presented only when the row's current
// host:api_port is the endpoint its credential row is pinned to, and both
// are set. A node whose API port was removed, or added after the pin, or
// whose host changed, is unconfirmed. The pin moves only through the
// kernel's own forward node writers, which an administrator's
// PUT /admin/forward/nodes/:id drives (Sync re-derives it from the row).
//
// Before the backfill a node has no credential row and so no pin, and a
// database without the split tables has none: the token is presented as
// before the split, and the read is counted
// (anixops_node_secrets_pin_total{reason="unpinned"}). The rule applies in
// every phase: the pin is the kernel's record, not a secret.
func ForwardNodeTokenAt(db *gorm.DB, node *model.ForwardNode) (string, error) {
	if node == nil {
		return "", nil
	}
	token := ForwardNodeToken(db, node)
	if token == "" {
		return "", nil
	}
	subject := subjectName(uint64(node.ID))
	if !SplitInstalled(db) {
		recordPin(PinUnpinned, subject)
		return token, nil
	}
	row, err := currentCredential(db, SubjectForward, uint64(node.ID), KindForwardNodeToken)
	if err != nil {
		return "", err
	}
	if row == nil {
		recordPin(PinUnpinned, subject)
		return token, nil
	}
	// endpointEqual is false when either endpoint is unset: a node without
	// an API port, now or when it was pinned, is unconfirmed.
	if !endpointEqual(row.Endpoint, ForwardNodeEndpoint(node)) {
		recordPin(PinUnconfirmed, subject)
		return "", ErrEndpointUnconfirmed
	}
	return token, nil
}

// endpointEqual compares two host:port endpoints as addresses: the same
// host spelling (the pin is derived from the row, so no normalization is
// wanted beyond the port's), and the same port.
func endpointEqual(pinned, current string) bool {
	pinnedHost, pinnedPort, err := net.SplitHostPort(pinned)
	if err != nil {
		return false
	}
	currentHost, currentPort, err := net.SplitHostPort(current)
	if err != nil {
		return false
	}
	pinnedNumber, err := strconv.Atoi(pinnedPort)
	if err != nil {
		return false
	}
	currentNumber, err := strconv.Atoi(currentPort)
	if err != nil {
		return false
	}
	return pinnedHost == currentHost && pinnedNumber == currentNumber && pinnedNumber > 0
}

// UnpinnedForwardNode is a forward node that holds a token but no API port:
// it has no endpoint, so its token is presented nowhere until an
// administrator sets the port (node-ops-service.md section 3.8, decided by
// the owner on 2026-10-01). Id and name only, never a value.
type UnpinnedForwardNode struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// ForwardNodesWithoutAPIPort lists the forward nodes that hold a token and
// have no API port, by id, for the node-secrets status command: the nodes
// whose gost backend changes and legacy rules fail ENDPOINT_UNCONFIRMED
// after the backfill.
func ForwardNodesWithoutAPIPort(ctx context.Context, db *gorm.DB) ([]UnpinnedForwardNode, error) {
	var rows []model.ForwardNode
	condition, args := ForwardNodeTokenPresent(db)
	if err := newSession(db).WithContext(ctx).Model(&model.ForwardNode{}).Select("id", "name").
		Where("(api_port IS NULL OR api_port <= 0)").Where(condition, args...).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	nodes := make([]UnpinnedForwardNode, 0, len(rows))
	for i := range rows {
		nodes = append(nodes, UnpinnedForwardNode{ID: rows[i].ID, Name: rows[i].Name})
	}
	return nodes, nil
}

// ForwardNodeTokenPresent answers a condition, with its arguments, for a
// query on v2_forward_node that holds for the forward nodes that have a
// token: the forward node inventory and the status command ask whether a
// node has one, not what it is.
//
//   - In phase dual_read and finalized, a node has a token when it has a
//     live credential row (forward_node_token, active, with a value): the
//     legacy column holds a tombstone once finalized.
//   - Before, the legacy column holds a usable value: neither empty, nor a
//     tombstone, nor the placeholder.
func ForwardNodeTokenPresent(db *gorm.DB) (string, []any) {
	if readsNew(db, TableForwardNode) && SplitInstalled(db) {
		return "EXISTS (SELECT 1 FROM v4_kernel_node_credential c WHERE c.subject_kind = ? AND c.subject_id = v2_forward_node.id " +
			"AND c.kind = ? AND c.status = ? AND c.value <> '')", []any{SubjectForward, KindForwardNodeToken, StatusActive}
	}
	return "(v2_forward_node.api_token IS NOT NULL AND v2_forward_node.api_token <> '' AND v2_forward_node.api_token NOT LIKE ? " +
		"AND v2_forward_node.api_token <> ?)", []any{tombstonePrefix + "%", Placeholder}
}

// ForwardNodeTokenAbsent is the negation of ForwardNodeTokenPresent.
func ForwardNodeTokenAbsent(db *gorm.DB) (string, []any) {
	condition, args := ForwardNodeTokenPresent(db)
	return "NOT " + condition, args
}

var pins counterSet // table, kind, reason

// recordPin counts a forward node token read that found no pin or an
// unconfirmed address, and logs the latter once per subject.
func recordPin(reason, subject string) {
	pins.add([3]string{TableForwardNode, KindForwardNodeToken, reason})
	if reason == PinUnconfirmed && firstLog("pin "+subject) {
		slog.Warn("forward node token not presented: the node's address is not its pinned endpoint",
			"component", "nodesecrets", "table", TableForwardNode, "subject", subject)
	}
}

// PinCount answers how many forward node token reads ended for reason in
// this process; an empty reason counts every reason.
func PinCount(reason string) uint64 {
	return pins.get(func(labels [3]string) bool {
		return reason == "" || labels[2] == reason
	})
}
