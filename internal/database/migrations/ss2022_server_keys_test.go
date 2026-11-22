package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stepIndex answers the position of the step id in history.
func stepIndex(t *testing.T, history []migrations.Step, id string) int {
	t.Helper()
	for i, step := range history {
		if step.ID() == id {
			return i
		}
	}
	t.Fatalf("no step %s", id)
	return -1
}

// 0041_ss2022_server_keys stores, for a Shadowsocks 2022 protocol without a
// server_key, the key Control used to derive from its node's created_at
// (the parent's for a relay), and leaves every other protocol alone.
func TestSS2022ServerKeysStepStoresTheFormerKey(t *testing.T) {
	forEachDialect(t, func(t *testing.T, open opener) {
		ctx := context.Background()
		history := steps(t)
		at := stepIndex(t, history, "0041_ss2022_server_keys")
		db := open(t)
		_, err := migrations.RunSteps(ctx, db, history[:at], migrateOptions())
		require.NoError(t, err)

		created := time.Unix(1782409707, 0).UTC()
		other := time.Unix(1700000000, 0).UTC()
		require.NoError(t, db.Exec(`INSERT INTO v5_node (id, name, created_at) VALUES (1, 'exit', ?), (2, 'relay', ?)`, created, other).Error)
		exec(t, db, `UPDATE v5_node SET parent_id = 1 WHERE id = 2`)
		exec(t, db, `INSERT INTO v5_node_protocol (id, node_id, type, settings) VALUES
			(10, 1, 'shadowsocks', '{"cipher":"2022-blake3-aes-256-gcm"}'),
			(11, 2, 'shadowsocks', '{"method":"2022-blake3-aes-128-gcm"}'),
			(12, 1, 'shadowsocks', '{"cipher":"2022-blake3-aes-256-gcm","server_key":"kept"}'),
			(13, 1, 'shadowsocks', '{"cipher":"aes-256-gcm"}'),
			(14, 1, 'vless', '{"cipher":"2022-blake3-aes-256-gcm"}')`)

		_, err = migrations.RunSteps(ctx, db, history[:at+1], migrateOptions())
		require.NoError(t, err)

		var secrets []struct {
			OwnerID uint64
			Value   string
		}
		require.NoError(t, db.Raw(`SELECT owner_id, value FROM v4_kernel_protocol_secret WHERE scope = 'node_protocol' AND json_pointer = '/server_key' ORDER BY owner_id`).Scan(&secrets).Error)
		require.Len(t, secrets, 2)
		assert.Equal(t, uint64(10), secrets[0].OwnerID)
		assert.Equal(t, `"YjI5ODBiNTkzYTM2N2MzZWQyNTlkMzA3YjUwYTEyYTk="`, secrets[0].Value)
		assert.Equal(t, uint64(11), secrets[1].OwnerID)
		assert.Equal(t, `"YjI5ODBiNTkzYTM2N2MzZQ=="`, secrets[1].Value, "a relay's key comes from its parent; aes-128 takes 16 characters")

		var settings []string
		require.NoError(t, db.Raw(`SELECT settings FROM v5_node_protocol ORDER BY id`).Scan(&settings).Error)
		assert.JSONEq(t, `{"cipher":"2022-blake3-aes-256-gcm","server_key":"********"}`, settings[0])
		assert.JSONEq(t, `{"method":"2022-blake3-aes-128-gcm","server_key":"********"}`, settings[1])
		assert.JSONEq(t, `{"cipher":"2022-blake3-aes-256-gcm","server_key":"kept"}`, settings[2])
		assert.JSONEq(t, `{"cipher":"aes-256-gcm"}`, settings[3])
		assert.JSONEq(t, `{"cipher":"2022-blake3-aes-256-gcm"}`, settings[4])
	})
}
