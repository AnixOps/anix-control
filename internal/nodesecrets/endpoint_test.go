package nodesecrets

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// endpointEqual compares addresses: the host as written, the port as a
// number, and never an unset port.
func TestEndpointEqual(t *testing.T) {
	cases := []struct {
		pinned, current string
		equal           bool
	}{
		{"relay.example:18080", "relay.example:18080", true},
		{"relay.example:18080", "relay.example:018080", true},
		{"relay.example:18080", "relay.example:18081", false},
		{"relay.example:18080", "other.example:18080", false},
		{"relay.example:18080", "Relay.example:18080", false},
		{"[2001:db8::1]:18080", "[2001:db8::1]:18080", true},
		{"[2001:db8::1]:18080", "[2001:db8::2]:18080", false},
		{"", "relay.example:18080", false},
		{"relay.example:18080", "", false},
		{"", "", false},
		{"relay.example:0", "relay.example:0", false},
		{"relay.example", "relay.example", false},
		{"relay.example:x", "relay.example:x", false},
	}
	for _, c := range cases {
		require.Equal(t, c.equal, endpointEqual(c.pinned, c.current), "%q vs %q", c.pinned, c.current)
	}
}

func pinnedNode(t *testing.T, db *gorm.DB, id uint, host string, apiPort int, token string) *model.ForwardNode {
	t.Helper()
	node := &model.ForwardNode{ID: id, Name: "relay", Host: host, Port: 443, APIPort: apiPort, APIToken: token}
	require.NoError(t, db.Create(node).Error)
	require.NoError(t, Sync(db, TableForwardNode, id))
	return node
}

// ForwardNodeTokenAt presents a token only at the pinned endpoint: a host
// or API port the row changed without the kernel's writer is unconfirmed,
// a node without an API port is pinned to nothing, the kernel's own write
// (Sync) moves the pin, and a node without a credential row is unpinned.
func TestForwardNodeTokenAt(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		before := PinCount(PinUnconfirmed)
		node := pinnedNode(t, db, 7, "relay.example", 18080, "relay-token")
		token, err := ForwardNodeTokenAt(db, node)
		require.NoError(t, err)
		require.Equal(t, "relay-token", token)

		// The row's address changes without Sync, as a package's write
		// would: nothing is presented.
		node.Host = "evil.example"
		_, err = ForwardNodeTokenAt(db, node)
		require.ErrorIs(t, err, ErrEndpointUnconfirmed)
		require.NotContains(t, err.Error(), "evil.example")
		require.NotContains(t, err.Error(), "relay-token")
		require.Equal(t, before+1, PinCount(PinUnconfirmed))

		node.Host, node.APIPort = "relay.example", 18081
		_, err = ForwardNodeTokenAt(db, node)
		require.ErrorIs(t, err, ErrEndpointUnconfirmed, "another API port is another endpoint")

		// The kernel's writer confirms the row as it is now.
		require.NoError(t, db.Save(node).Error)
		require.NoError(t, Sync(db, TableForwardNode, node.ID))
		token, err = ForwardNodeTokenAt(db, node)
		require.NoError(t, err)
		require.Equal(t, "relay-token", token)

		// A node without an API port is pinned to nothing: the token is
		// presented nowhere, whether the port was removed by a package or
		// by the kernel, and a port a package adds does not confirm.
		node.APIPort = 0
		_, err = ForwardNodeTokenAt(db, node)
		require.ErrorIs(t, err, ErrEndpointUnconfirmed)
		require.NoError(t, db.Save(node).Error)
		require.NoError(t, Sync(db, TableForwardNode, node.ID))
		_, err = ForwardNodeTokenAt(db, node)
		require.ErrorIs(t, err, ErrEndpointUnconfirmed, "no API port, no endpoint")
		node.APIPort = 18080
		_, err = ForwardNodeTokenAt(db, node)
		require.ErrorIs(t, err, ErrEndpointUnconfirmed, "a port the kernel did not write")
		require.NoError(t, db.Save(node).Error)
		require.NoError(t, Sync(db, TableForwardNode, node.ID))
		token, err = ForwardNodeTokenAt(db, node)
		require.NoError(t, err)
		require.Equal(t, "relay-token", token)

		// A changed token keeps the pin of its row.
		node.APIToken = "rotated-token"
		require.NoError(t, db.Save(node).Error)
		require.NoError(t, Sync(db, TableForwardNode, node.ID))
		token, err = ForwardNodeTokenAt(db, node)
		require.NoError(t, err)
		require.Equal(t, "rotated-token", token)

		// A node without a credential row (not backfilled) is unpinned and
		// presented as before the split, counted.
		unpinned := &model.ForwardNode{ID: 8, Name: "old", Host: "old.example", Port: 443, APIPort: 18080, APIToken: "old-token"}
		require.NoError(t, db.Create(unpinned).Error)
		unpinnedBefore := PinCount(PinUnpinned)
		token, err = ForwardNodeTokenAt(db, unpinned)
		require.NoError(t, err)
		require.Equal(t, "old-token", token)
		require.Equal(t, unpinnedBefore+1, PinCount(PinUnpinned))

		// No token: nothing to present, nothing to pin.
		empty := &model.ForwardNode{ID: 9, Name: "none", Host: "none.example", Port: 443, APIPort: 18080}
		require.NoError(t, db.Create(empty).Error)
		token, err = ForwardNodeTokenAt(db, empty)
		require.NoError(t, err)
		require.Empty(t, token)
		token, err = ForwardNodeTokenAt(db, nil)
		require.NoError(t, err)
		require.Empty(t, token)
	})
}

// A database without the split tables pins nothing.
func TestForwardNodeTokenAtWithoutTheSplitTables(t *testing.T) {
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(&model.ForwardNode{}))
	node := &model.ForwardNode{ID: 7, Name: "relay", Host: "relay.example", Port: 443, APIPort: 18080, APIToken: "relay-token"}
	require.NoError(t, db.Create(node).Error)
	require.False(t, SplitInstalled(db))
	token, err := ForwardNodeTokenAt(db, node)
	require.NoError(t, err)
	require.Equal(t, "relay-token", token)
}

// ForwardNodeEndpoint is host:api_port, or nothing without either.
func TestForwardNodeEndpoint(t *testing.T) {
	require.Equal(t, "relay.example:18080", ForwardNodeEndpoint(&model.ForwardNode{Host: "relay.example", APIPort: 18080}))
	require.Equal(t, "[2001:db8::1]:18080", ForwardNodeEndpoint(&model.ForwardNode{Host: "2001:db8::1", APIPort: 18080}))
	require.Empty(t, ForwardNodeEndpoint(&model.ForwardNode{Host: "relay.example"}))
	require.Empty(t, ForwardNodeEndpoint(&model.ForwardNode{APIPort: 18080}))
	require.Empty(t, ForwardNodeEndpoint(nil))
}
