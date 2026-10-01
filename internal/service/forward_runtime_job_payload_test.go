package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The old payload shape: the ingress node's token copied into the job.
const oldPayload = `{"resourceType":"panel_forward","backend":"clean_agent","action":"create","panelForward":{"forward":{"id":40,"name":"a"},"ingressNode":{"id":10,"name":"relay","host":"198.51.100.10","port":22,"apiPort":18080,"apiToken":"relay-token-10"}}}`

func TestScrubForwardRuntimeJobPayload(t *testing.T) {
	scrubbed, changed := ScrubForwardRuntimeJobPayload(oldPayload)
	require.True(t, changed)
	require.NotContains(t, scrubbed, "relay-token-10")
	require.NotContains(t, scrubbed, "apiToken", "the key is removed, as a new payload omits it")
	require.Contains(t, scrubbed, `"host":"198.51.100.10"`, "everything else stays")
	require.Contains(t, scrubbed, `"apiPort":18080`)
	again, changed := ScrubForwardRuntimeJobPayload(scrubbed)
	require.False(t, changed, "idempotent")
	require.Equal(t, scrubbed, again)

	// Every secret key, at any depth, in strings, arrays and objects.
	nested := `{"legacyRule":{"relayNode":{"apiToken":"t1"},"exitNode":{"api_token":"t2","password":["p"],"secret":{"k":"v"}}},"list":[{"token":"t3"},{"token":""}]}`
	scrubbed, changed = ScrubForwardRuntimeJobPayload(nested)
	require.True(t, changed)
	for _, value := range []string{"t1", "t2", "t3", `["p"]`, `{"k":"v"}`} {
		require.NotContains(t, scrubbed, value)
	}
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(scrubbed), &decoded))
	require.NotContains(t, decoded["legacyRule"].(map[string]any)["relayNode"], "apiToken")
	require.Len(t, decoded["list"], 2)

	// An empty or null value under a secret key goes too.
	for _, payload := range []string{`{"apiToken":""}`, `{"apiToken":null}`} {
		scrubbed, changed := ScrubForwardRuntimeJobPayload(payload)
		require.True(t, changed, payload)
		require.Equal(t, "{}", scrubbed)
	}

	// Payloads with nothing to scrub, and ones that are not JSON, are left
	// as they are.
	for _, payload := range []string{"", "  ", `{"action":"create","panelForward":{"ingressNode":{"id":10}}}`, "not json", `{"a":1} trailing`} {
		same, changed := ScrubForwardRuntimeJobPayload(payload)
		require.False(t, changed, payload)
		require.Equal(t, payload, same)
	}
}

func openScrubDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "jobs.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.ForwardRuntimeJob{}))
	return db
}

// The start-up pass rewrites the rows written before NO-7 that carry a
// token, in batches, leaves every other row alone, and finds nothing the
// second time.
func TestScrubForwardRuntimeJobPayloads(t *testing.T) {
	db := openScrubDB(t)
	forward := uint(40)
	rows := []model.ForwardRuntimeJob{
		{Backend: "clean_agent", Action: "create", ForwardID: &forward, Payload: oldPayload},
		{Backend: "gost", Action: "update", ForwardID: &forward, Payload: strings.ReplaceAll(oldPayload, "clean_agent", "gost")},
		{Backend: "clean_agent", Action: "delete", ForwardID: &forward, Payload: `{"action":"delete","panelForward":{"ingressNode":{"id":10,"apiToken":""}}}`},
		{Backend: "gost", Action: "sync", ForwardID: &forward, Payload: `{"action":"sync","panelForward":{"ingressNode":{"id":10}}}`},
		{Backend: "nftables_ansible", Action: "create", ForwardID: &forward, Payload: `{"action":"create","node":{"id":12,"host":"198.51.100.12"}}`},
		{Backend: "gost", Action: "create", ForwardID: &forward, Payload: ""},
		{Backend: "gost", Action: "create", ForwardID: &forward, Payload: `{"legacyRule":{"relayNode":{"apiToken":"relay-token-10"},"exitNode":{"apiToken":"exit-token-12"}}}`},
	}
	require.NoError(t, db.Create(&rows).Error)
	untouched := map[uint]string{rows[3].ID: rows[3].Payload, rows[4].ID: rows[4].Payload, rows[5].ID: rows[5].Payload}

	// Four rows name the key (one with an empty value); batches of three
	// leave one for a second batch.
	scrubbed, err := ScrubForwardRuntimeJobPayloads(context.Background(), db, 3)
	require.NoError(t, err)
	require.EqualValues(t, 4, scrubbed)
	var stored []model.ForwardRuntimeJob
	require.NoError(t, db.Order("id").Find(&stored).Error)
	for _, row := range stored {
		require.NotContains(t, row.Payload, "token-1", "row %d", row.ID)
		require.NotContains(t, row.Payload, "apiToken", "row %d", row.ID)
		if before, ok := untouched[row.ID]; ok {
			require.Equal(t, before, row.Payload, "row %d is left as it was", row.ID)
		}
	}
	require.Contains(t, stored[0].Payload, `"host":"198.51.100.10"`)

	scrubbed, err = ScrubForwardRuntimeJobPayloads(context.Background(), db, 0)
	require.NoError(t, err)
	require.Zero(t, scrubbed, "nothing left to scrub")

	// A cancelled context stops the pass.
	require.NoError(t, db.Create(&model.ForwardRuntimeJob{Backend: "gost", Action: "create", ForwardID: &forward, Payload: oldPayload}).Error)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ScrubForwardRuntimeJobPayloads(ctx, db, 10)
	require.ErrorIs(t, err, context.Canceled)
	_, err = ScrubForwardRuntimeJobPayloads(context.Background(), nil, 10)
	require.Error(t, err)
	RunForwardRuntimeJobPayloadScrub(context.Background(), db)
	scrubbed, err = ScrubForwardRuntimeJobPayloads(context.Background(), db, 10)
	require.NoError(t, err)
	require.Zero(t, scrubbed)
}

// The job list serves old rows scrubbed until the pass rewrote them.
func TestListRuntimeJobsServesNoToken(t *testing.T) {
	db := openScrubDB(t)
	forward := uint(40)
	require.NoError(t, db.Create(&model.ForwardRuntimeJob{Backend: "clean_agent", Action: "create", ForwardID: &forward, Payload: oldPayload}).Error)
	jobs, err := NewPanelForwardService(db).ListRuntimeJobs(PanelRuntimeJobFilter{})
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.NotContains(t, jobs[0].Payload, "relay-token-10")
	require.Contains(t, jobs[0].Payload, `"host":"198.51.100.10"`)
}
