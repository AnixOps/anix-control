package realitytarget

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

func autoFixture(t *testing.T) (*gorm.DB, *AutoWorker, *time.Time, fakeProber) {
	t.Helper()
	db := testDB(t)
	if err := db.AutoMigrate(&model.RealityTargetAuto{}, &model.OperationLog{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Node{ID: 57, Name: "DE", APIKey: "k57", LastPublicAddress: "82.139.194.175"}).Error; err != nil {
		t.Fatal(err)
	}
	protocol := model.NodeProtocol{ID: 41, NodeID: 57, Name: "DE Reality", TLS: 2,
		RealitySettings: text(`{"dest":"www.microsoft.com:443","server_name":"www.microsoft.com","private_key":"sealed"}`),
		TLSSettings:     text(`{"server_name":"www.microsoft.com","fingerprint":"chrome"}`)}
	if err := db.Create(&protocol).Error; err != nil {
		t.Fatal(err)
	}
	prober := fakeProber{
		answers: map[string]ProbeResult{
			"www.microsoft.com": {Verdict: VerdictFailed, Failures: []string{"reality"}},
			"www.apple.com":     {Verdict: VerdictOK, Address: "82.139.194.20:443", HandshakeMS: 4, Curve: "X25519"},
		},
		errs: map[string]error{},
	}
	now := time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)
	service := &Service{DB: db, Prober: prober, Now: func() time.Time { return now }}
	worker := &AutoWorker{
		Service: service,
		Ready:   func(node agentcontrol.AgentNode) bool { return node.ID == 57 },
		Update: func(id uint, updates map[string]any) error {
			return db.Model(&model.NodeProtocol{}).Where("id = ?", id).Updates(updates).Error
		},
	}
	return db, worker, &now, prober
}

func autoRow(t *testing.T, db *gorm.DB) model.RealityTargetAuto {
	t.Helper()
	var row model.RealityTargetAuto
	if err := db.First(&row, "node_id = ?", 57).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestAutoSwitchesAfterTwoFailures(t *testing.T) {
	db, worker, now, _ := autoFixture(t)
	ctx := context.Background()

	// Off: nothing runs.
	if err := worker.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	var probes int64
	db.Model(&model.RealityTargetProbe{}).Count(&probes)
	if probes != 0 {
		t.Fatal("a node without automatic selection was probed")
	}

	state, err := worker.Service.SetAuto(ctx, 57, true)
	if err != nil || !state.Enabled || state.NextRunAt != nil {
		t.Fatalf("enable: %+v %v", state, err)
	}

	// First failure: no switch, a recheck in AutoRecheckDelay.
	if err := worker.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	row := autoRow(t, db)
	if row.FailStreak != 1 || row.NextRunAt == nil || !row.NextRunAt.Equal(now.Add(AutoRecheckDelay)) || !strings.Contains(row.LastOutcome, "rechecking") {
		t.Fatalf("after one failure: %+v", row)
	}
	var protocol model.NodeProtocol
	db.First(&protocol, 41)
	if !strings.Contains(*protocol.RealitySettings, "www.microsoft.com") {
		t.Fatal("switched after one failure")
	}

	// Not due yet: nothing happens.
	*now = now.Add(time.Minute)
	if err := worker.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if autoRow(t, db).FailStreak != 1 {
		t.Fatal("ran before it was due")
	}

	// Second failure: switched to the recommended target, audited.
	*now = now.Add(AutoRecheckDelay)
	if err := worker.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	row = autoRow(t, db)
	if row.FailStreak != 0 || row.LastSwitchAt == nil || !row.NextRunAt.Equal(now.Add(AutoInterval)) || !strings.Contains(row.LastOutcome, "www.microsoft.com → www.apple.com") {
		t.Fatalf("after two failures: %+v", row)
	}
	db.First(&protocol, 41)
	var reality, tlsSettings map[string]any
	_ = json.Unmarshal([]byte(*protocol.RealitySettings), &reality)
	_ = json.Unmarshal([]byte(*protocol.TLSSettings), &tlsSettings)
	if reality["dest"] != "www.apple.com:443" || reality["server_name"] != "www.apple.com" || reality["private_key"] != "sealed" ||
		tlsSettings["server_name"] != "www.apple.com" || tlsSettings["fingerprint"] != "chrome" {
		t.Fatalf("protocol: %v %v", reality, tlsSettings)
	}
	var audit model.OperationLog
	if err := db.Where("action = ?", AutoAuditAction).First(&audit).Error; err != nil || audit.TargetID == nil || *audit.TargetID != 57 || !strings.Contains(audit.Content, `"to":"www.apple.com:443"`) {
		t.Fatalf("audit %+v %v", audit, err)
	}

	// The new target passes: the next run keeps it.
	*now = now.Add(AutoInterval)
	if err := worker.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	row = autoRow(t, db)
	if row.FailStreak != 0 || row.LastOutcome != "every current target passed" {
		t.Fatalf("after the switch: %+v", row)
	}
}

func TestAutoKeepsATargetThatDidNotAnswer(t *testing.T) {
	db, worker, now, prober := autoFixture(t)
	ctx := context.Background()
	prober.errs["www.microsoft.com"] = context.DeadlineExceeded
	if _, err := worker.Service.SetAuto(ctx, 57, true); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := worker.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(AutoInterval)
	}
	row := autoRow(t, db)
	if row.FailStreak != 0 || row.LastSwitchAt != nil {
		t.Fatalf("a probe without an answer counted as a failure: %+v", row)
	}
}

func TestAutoWaitsForTheAgent(t *testing.T) {
	db, worker, now, _ := autoFixture(t)
	ctx := context.Background()
	worker.Ready = func(agentcontrol.AgentNode) bool { return false }
	if _, err := worker.Service.SetAuto(ctx, 57, true); err != nil {
		t.Fatal(err)
	}
	if err := worker.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	row := autoRow(t, db)
	if !row.NextRunAt.Equal(now.Add(AutoRetryDelay)) || !strings.Contains(row.LastOutcome, "cannot probe") {
		t.Fatalf("%+v", row)
	}
	var probes int64
	db.Model(&model.RealityTargetProbe{}).Count(&probes)
	if probes != 0 {
		t.Fatal("probed without a ready Agent")
	}

	// Turning it off and on again makes it due at once.
	if _, err := worker.Service.SetAuto(ctx, 57, false); err != nil {
		t.Fatal(err)
	}
	state, err := worker.Service.SetAuto(ctx, 57, true)
	if err != nil || state.NextRunAt != nil || state.LastOutcome == "" {
		t.Fatalf("%+v %v", state, err)
	}
}
