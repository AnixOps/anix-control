package reachprobe

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// fakeProber answers, per vantage, which hosts it reaches.
type fakeProber struct {
	reach map[uint32]map[string]bool
	calls map[uint32][]kernelnodeops.ReachTarget
}

func (f *fakeProber) Capable(node agentcontrol.AgentNode) bool { return f.reach[node.ID] != nil }

func (f *fakeProber) Probe(_ context.Context, node agentcontrol.AgentNode, targets []kernelnodeops.ReachTarget, _ time.Duration) (json.RawMessage, error) {
	f.calls[node.ID] = append(f.calls[node.ID], targets...)
	answer := map[string]any{}
	list := []any{}
	for _, t := range targets {
		list = append(list, map[string]any{"host": t.Host, "reachable": f.reach[node.ID][t.Host]})
	}
	answer["targets"] = list
	return json.Marshal(answer)
}

func text(s string) *string { return &s }

func TestRoundAndStatuses(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.ReachVantage{}, &model.ReachProbe{}); err != nil {
		t.Fatal(err)
	}
	parent := uint(3)
	nodes := []model.Node{
		{ID: 1, Name: "domestic", Host: "1.1.1.1", Status: model.NodeStatusOnline},
		{ID: 2, Name: "overseas", Host: "2.2.2.2", Status: model.NodeStatusOnline},
		{ID: 3, Name: "exit-a", Host: "a.example", ServerIP: text("3.3.3.3"), Status: model.NodeStatusOnline},
		{ID: 4, Name: "exit-b", Host: "4.4.4.4:8443", Status: model.NodeStatusOnline},
		{ID: 5, Name: "relay", Host: "5.5.5.5", ParentID: &parent, Status: model.NodeStatusOnline},
		{ID: 6, Name: "private", Host: "10.0.0.6", Status: model.NodeStatusOnline},
		{ID: 7, Name: "disabled", Host: "7.7.7.7", Status: model.NodeStatusDisabled},
	}
	if err := db.Create(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	var protocols []model.NodeProtocol
	for id := uint(1); id <= 7; id++ {
		for _, port := range []int{443, 8443, 22, 80} {
			protocols = append(protocols, model.NodeProtocol{NodeID: id, Port: port, Enable: 1})
		}
	}
	if err := db.Create(&protocols).Error; err != nil {
		t.Fatal(err)
	}
	prober := &fakeProber{
		reach: map[uint32]map[string]bool{
			1: {"2.2.2.2": true, "3.3.3.3": true, "4.4.4.4": false},
			2: {"1.1.1.1": true, "3.3.3.3": true, "4.4.4.4": true},
		},
		calls: map[uint32][]kernelnodeops.ReachTarget{},
	}
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	s := &Service{DB: db, Prober: prober, Now: func() time.Time { return now }}
	ctx := context.Background()

	if err := s.SetVantages(ctx, []Vantage{{NodeID: 1, Region: "elsewhere"}}); err == nil {
		t.Fatal("an unknown region was accepted")
	}
	if err := s.SetVantages(ctx, []Vantage{{NodeID: 99, Region: RegionDomestic}}); err == nil {
		t.Fatal("a missing node was accepted")
	}
	if err := s.SetVantages(ctx, []Vantage{{NodeID: 1, Region: RegionDomestic}, {NodeID: 2, Region: RegionOverseas}}); err != nil {
		t.Fatal(err)
	}
	if ran, err := s.RunRound(ctx); err != nil || ran != 2 {
		t.Fatalf("ran %d, %v", ran, err)
	}
	// The vantage skips itself, relays, private and disabled nodes; ports
	// are the lowest three enabled ones.
	got := prober.calls[1]
	if len(got) != 3 || got[0].Host != "2.2.2.2" || got[1].Host != "3.3.3.3" || got[2].Host != "4.4.4.4" {
		t.Fatalf("vantage 1 probed %+v", got)
	}
	if ports := got[0].Ports; len(ports) != 3 || ports[0] != 22 || ports[2] != 443 {
		t.Fatalf("ports %v", ports)
	}

	statuses, err := s.Statuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint]string{1: StatusOK, 2: StatusOK, 3: StatusOK, 4: StatusRegionUnreachable}
	if len(statuses) != len(want) {
		t.Fatalf("statuses %+v", statuses)
	}
	for _, status := range statuses {
		if status.Status != want[status.NodeID] {
			t.Fatalf("node %d: %s, want %s", status.NodeID, status.Status, want[status.NodeID])
		}
	}

	// Stale answers no longer count; dropping a vantage drops its answers.
	now = now.Add(Stale + time.Minute)
	if statuses, _ := s.Statuses(ctx); len(statuses) != 0 {
		t.Fatalf("stale answers counted: %+v", statuses)
	}
	if err := s.SetVantages(ctx, []Vantage{{NodeID: 2, Region: RegionOverseas}}); err != nil {
		t.Fatal(err)
	}
	var left int64
	db.Model(&model.ReachProbe{}).Where("vantage_node_id = 1").Count(&left)
	if left != 0 {
		t.Fatalf("%d answers of a removed vantage are left", left)
	}
}

func TestVerdict(t *testing.T) {
	yes, no := true, false
	for _, c := range []struct {
		domestic, overseas *bool
		want               string
	}{
		{nil, nil, StatusUnknown},
		{&yes, &yes, StatusOK},
		{&no, &yes, StatusRegionUnreachable},
		{&yes, &no, StatusOverseasUnreachable},
		{&no, &no, StatusUnreachable},
		{nil, &no, StatusUnreachable},
		{&yes, nil, StatusOK},
	} {
		if got := verdict(c.domestic, c.overseas); got != c.want {
			t.Fatalf("%v %v: %s, want %s", c.domestic, c.overseas, got, c.want)
		}
	}
}
