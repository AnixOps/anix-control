package realitytarget

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestParseCandidate(t *testing.T) {
	for raw, want := range map[string]Candidate{
		"www.Example.com":     {ServerName: "www.example.com", Port: 443},
		" example.com.:8443 ": {ServerName: "example.com", Port: 8443},
	} {
		got, err := ParseCandidate(raw)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "1.2.3.4", "[::1]:443", "localhost", "a_b.example.com", "example.com:0", "example.com:x"} {
		if _, err := ParseCandidate(raw); err == nil {
			t.Errorf("%q: want an error", raw)
		}
	}
}

func TestCandidates(t *testing.T) {
	list, err := Candidates([]string{"own.example.com", "www.microsoft.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].ServerName != "own.example.com" || list[1].ServerName != "www.microsoft.com" || len(list) != len(DefaultCandidates)+1 {
		t.Fatalf("%v", list)
	}
	if list, err := Candidates([]string{"own.example.com"}, false); err != nil || len(list) != 1 {
		t.Fatalf("%v %v", list, err)
	}
	if _, err := Candidates(nil, false); err == nil {
		t.Fatal("no candidates: want an error")
	}
	if _, err := Candidates([]string{"10.0.0.1"}, true); err == nil {
		t.Fatal("an address: want an error")
	}
}

func TestScore(t *testing.T) {
	node := netip.MustParseAddr("82.139.194.175")
	pass := ProbeResult{Verdict: VerdictOK, Address: "82.139.194.20:443", HandshakeMS: 8, Curve: "X25519MLKEM768", OCSPStapled: true, CertChainBytes: 4000}
	if got := Score(pass, ScoreInput{NodeAddr: node}); got != 80 {
		t.Fatalf("neighbour: %d", got)
	}
	far := ProbeResult{Verdict: VerdictOK, Address: "[2600::1]:443", HandshakeMS: 120, Curve: "X25519"}
	if got := Score(far, ScoreInput{NodeAddr: node}); got != 10 {
		t.Fatalf("far: %d", got)
	}
	wide := pass
	wide.Address = "82.139.1.1:443"
	if got := Score(wide, ScoreInput{NodeAddr: node, UsedBy: 2}); got != 55 {
		t.Fatalf("same /16, used twice: %d", got)
	}
	cdn := far
	cdn.CDN = "cloudflare"
	if got := Score(cdn, ScoreInput{NodeAddr: node}); got != 0 {
		t.Fatalf("cdn floors at 0: %d", got)
	}
	failed := pass
	failed.Verdict = VerdictFailed
	if got := Score(failed, ScoreInput{NodeAddr: node}); got != 0 {
		t.Fatalf("failed: %d", got)
	}
}

type fakeProber struct {
	answers map[string]ProbeResult
	errs    map[string]error
}

func (f fakeProber) Probe(_ context.Context, node agentcontrol.AgentNode, serverName string, port int, timeout time.Duration) (json.RawMessage, error) {
	if node.Kind != agentcontrol.NodeKindProxy || node.ID != 57 || port != 443 || timeout != DefaultTimeout {
		return nil, errors.New("unexpected call")
	}
	if err := f.errs[serverName]; err != nil {
		return nil, err
	}
	return json.Marshal(f.answers[serverName])
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.RealityTargetProbe{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func text(s string) *string { return &s }

func TestRunAndReport(t *testing.T) {
	db := testDB(t)
	if err := db.Create(&model.Node{ID: 57, Name: "DE", APIKey: "k57", Host: "de.example.net", LastPublicAddress: "82.139.194.175"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Node{ID: 29, Name: "HK", APIKey: "k29", Host: "203.0.113.9"}).Error; err != nil {
		t.Fatal(err)
	}
	protocols := []model.NodeProtocol{
		{NodeID: 57, Name: "DE Reality", TLS: 2, RealitySettings: text(`{"dest":"www.microsoft.com:443","server_name":"www.microsoft.com","private_key":"sealed"}`)},
		{NodeID: 29, Name: "HK Reality", TLS: 2, RealitySettings: text(`{"dest":"near.example.com:443","server_names":["near.example.com"]}`)},
		{NodeID: 29, Name: "HK SS", TLS: 0},
	}
	if err := db.Create(&protocols).Error; err != nil {
		t.Fatal(err)
	}
	prober := fakeProber{
		answers: map[string]ProbeResult{
			"near.example.com":  {Verdict: VerdictOK, Address: "82.139.194.9:443", HandshakeMS: 3, Curve: "X25519"},
			"www.microsoft.com": {Verdict: VerdictOK, Address: "23.1.2.3:443", HandshakeMS: 30, Curve: "X25519MLKEM768", CDN: "akamai"},
			"bad.example.com":   {Verdict: VerdictFailed, Failures: []string{"h2"}},
		},
		errs: map[string]error{"slow.example.com": context.DeadlineExceeded},
	}
	service := &Service{DB: db, Prober: prober}
	candidates, err := Candidates([]string{"near.example.com", "www.microsoft.com", "bad.example.com", "slow.example.com"}, false)
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Run(context.Background(), 57, candidates, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.NodeAddress != "82.139.194.175" || len(report.Current) != 1 || report.Current[0].ServerName != "www.microsoft.com" {
		t.Fatalf("node: %+v", report)
	}
	order := []string{}
	for _, target := range report.Targets {
		order = append(order, target.ServerName+":"+target.Verdict)
	}
	want := "near.example.com:ok www.microsoft.com:ok bad.example.com:failed slow.example.com:error"
	if got := join(order); got != want {
		t.Fatalf("order %q", got)
	}
	// near: same /24 30 + fast 30 - used by HK 5 = 55.
	if report.Recommended == nil || report.Recommended.ServerName != "near.example.com" || report.Recommended.Score != 55 || report.Recommended.UsedBy != 1 {
		t.Fatalf("recommended %+v", report.Recommended)
	}
	if report.Targets[3].Error == "" {
		t.Fatal("the probe's error is answered")
	}

	// A later run that fails keeps the stored rows; Report reads them.
	prober.errs["near.example.com"] = errors.New("offline")
	if _, err := service.Run(context.Background(), 57, candidates[:1], 0); err != nil {
		t.Fatal(err)
	}
	stored, err := service.Report(context.Background(), 57)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Targets) != 3 || stored.Recommended == nil || stored.Recommended.ServerName != "near.example.com" || stored.Targets[0].Result == nil {
		t.Fatalf("stored %+v", stored)
	}
	var count int64
	db.Model(&model.RealityTargetProbe{}).Count(&count)
	if count != 3 {
		t.Fatalf("rows %d", count)
	}
}

func join(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += " "
		}
		out += item
	}
	return out
}
