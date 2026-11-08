package nodesecretsplit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file tests the views of the split's remainder (section 4.6).

// secretsOf lists every secret value of the fleet.
func (f *fleet) secretsOf(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var wireGuard model.NodeProtocol
	require.NoError(t, db.First(&wireGuard, f.wireGuard.ID).Error)
	nodesecrets.ResolveProtocol(db, &wireGuard)
	values := []string{
		f.rawPrivate, f.realityPrivate,
		f.peer.PrivateKey, f.peer.PresharedKey,
	}
	for _, position := range nodesecrets.SecretPositions(*wireGuard.Settings) {
		values = append(values, strings.Trim(position.Value, `"`))
	}
	for _, value := range values {
		require.NotEmpty(t, value)
	}
	return values
}

// splitViews are the views of section 4.6.
var splitViews = []string{
	"kapi_node_public_v1", "kapi_node_protocol_public_v1", "kapi_wireguard_peer_v1",
}

// movedColumns are the columns no view may show.
var movedColumns = map[string]bool{
	"api_key": true, "api_key_hash": true, "secret": true, "key": true, "key_hash": true, "api_token": true,
	"token": true, "private_key": true, "preshared_key": true, "value": true,
}

func viewExists(t *testing.T, db *gorm.DB, view string) bool {
	t.Helper()
	query := "SELECT count(*) FROM sqlite_master WHERE type = 'view' AND name = ?"
	if db.Name() == "postgres" {
		query = "SELECT count(*) FROM pg_views WHERE schemaname = current_schema() AND viewname = ?"
	}
	var count int64
	require.NoError(t, db.Raw(query, view).Scan(&count).Error)
	return count > 0
}

// TestSplitViewsShowNoSecret: the views of the split's remainder exist and
// show no moved column and no secret.
func TestSplitViewsShowNoSecret(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b backend) {
		db := b.db
		f := newFleet(t, db)
		secrets := f.secretsOf(t, db)
		require.NoError(t, packagestore.EnsureKernelAPIViews(db))
		for _, view := range splitViews {
			require.True(t, viewExists(t, db, view), view)
			var rows []map[string]any
			require.NoError(t, db.Table(view).Find(&rows).Error)
			require.NotEmpty(t, rows, view)
			for _, row := range rows {
				for column, value := range row {
					require.False(t, movedColumns[column], "%s shows %s", view, column)
					text := fmt.Sprint(value)
					if bytes, ok := value.([]byte); ok {
						text = string(bytes)
					}
					for _, secret := range secrets {
						require.NotContains(t, text, secret, "%s.%s", view, column)
					}
					require.False(t, nodesecrets.IsTombstone(text) && column != "raw_config", "%s.%s", view, column)
				}
			}
		}
	})
}
