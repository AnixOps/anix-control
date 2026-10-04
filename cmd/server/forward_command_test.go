package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestForwardCommandResetsNodeGeneration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(append(model.KernelForwardModels(), &model.OperationLog{})...))
	now := time.Now().UTC()
	require.NoError(t, db.Create(&model.KernelForwardNodeState{
		NodeRef: "forward-7", Generation: 3, StateHash: "abc",
		StateJSON: `{"node_ref":"forward-7","generation":3,"state_hash":"abc"}`, PlannedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.KernelForwardNodeReport{
		NodeRef: "forward-7", Generation: 9, StateHash: "def", ReportJSON: `{}`, ObservedAt: now, ReceivedAt: now,
	}).Error)
	ctx := context.Background()
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, nil, db, append([]string{"forward"}, arguments...), &output)
		return output.String(), err
	}

	for _, arguments := range [][]string{{}, {"reset-node"}, {"bogus", "forward-7"}, {"reset-node", "forward-7", "extra"}} {
		_, err := run(arguments...)
		require.ErrorContains(t, err, "invalid forward command", arguments)
	}
	_, err = run("reset-node", "forward-8")
	require.ErrorIs(t, err, kernelforward.ErrNoState)
	_, err = run("reset-node", "node-7")
	require.ErrorIs(t, err, kernelforward.ErrInvalidRequest)

	output, err := run("reset-node", "forward-7")
	require.NoError(t, err)
	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &printed))
	assert.Equal(t, map[string]any{
		"node_ref": "forward-7", "previous_generation": float64(3), "reported_generation": float64(9),
		"generation": float64(10), "state_hash": "abc", "audited": true,
	}, printed)
	state, found, err := kernelforward.New(db).State(ctx, "forward-7")
	require.NoError(t, err)
	require.True(t, found)
	assert.EqualValues(t, 10, state.GetGeneration())
	var entry model.OperationLog
	require.NoError(t, db.First(&entry, "action = ?", "forward.reset_node").Error)
	assert.Equal(t, "system/cli", entry.Username)
	assert.Contains(t, entry.Content, `"generation":10`)
}
