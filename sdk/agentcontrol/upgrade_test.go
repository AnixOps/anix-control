package agentcontrol

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func validUpgradeRequest() UpgradeRequest {
	return UpgradeRequest{
		Schema: UpgradeSchemaV1, CampaignID: "c1", Action: UpgradeActionUpgrade, TargetVersion: "v4.2.0", PreviousVersion: "v4.1.0",
		Artifacts: []UpgradeArtifact{{
			Arch: "amd64", Asset: "anix-agent-linux-64.zip", URL: "https://control.example/install/agent/v4.2.0/anix-agent-linux-64.zip",
			SHA256: strings.Repeat("a", 64), Size: 1024, Signature: base64.StdEncoding.EncodeToString(make([]byte, 64)),
		}},
	}
}

func TestParseUpgradeRequest(t *testing.T) {
	data, err := json.Marshal(validUpgradeRequest())
	if err != nil {
		t.Fatal(err)
	}
	request, err := ParseUpgradeRequest(data)
	if err != nil {
		t.Fatalf("ParseUpgradeRequest() = %v", err)
	}
	if artifact, ok := request.Artifact("amd64"); !ok || artifact.Size != 1024 {
		t.Fatalf("Artifact(amd64) = %+v, %v", artifact, ok)
	}
	if _, ok := request.Artifact("arm64"); ok {
		t.Fatal("Artifact(arm64) found")
	}
	// Unknown fields are additions a newer Control may send.
	if _, err := ParseUpgradeRequest([]byte(strings.Replace(string(data), `{"schema"`, `{"later":1,"schema"`, 1))); err != nil {
		t.Fatalf("unknown field refused: %v", err)
	}
	rollback := UpgradeRequest{Schema: UpgradeSchemaV1, CampaignID: "c1", Action: UpgradeActionRollback, TargetVersion: "v4.1.0"}
	if err := rollback.Validate(); err != nil {
		t.Fatalf("rollback refused: %v", err)
	}
}

func TestParseUpgradeRequestRefuses(t *testing.T) {
	for name, mutate := range map[string]func(*UpgradeRequest){
		"schema":          func(r *UpgradeRequest) { r.Schema = "anixops.agent-upgrade/v2" },
		"campaign":        func(r *UpgradeRequest) { r.CampaignID = "" },
		"action":          func(r *UpgradeRequest) { r.Action = "install" },
		"target":          func(r *UpgradeRequest) { r.TargetVersion = "4.2.0" },
		"previous":        func(r *UpgradeRequest) { r.PreviousVersion = "latest" },
		"no artifacts":    func(r *UpgradeRequest) { r.Artifacts = nil },
		"http url":        func(r *UpgradeRequest) { r.Artifacts[0].URL = "http://control.example/x.zip" },
		"digest":          func(r *UpgradeRequest) { r.Artifacts[0].SHA256 = strings.Repeat("A", 64) },
		"size":            func(r *UpgradeRequest) { r.Artifacts[0].Size = 0 },
		"signature":       func(r *UpgradeRequest) { r.Artifacts[0].Signature = "c2ln" },
		"asset":           func(r *UpgradeRequest) { r.Artifacts[0].Asset = "../anix-agent" },
		"repeated arch":   func(r *UpgradeRequest) { r.Artifacts = append(r.Artifacts, r.Artifacts[0]) },
		"rollback assets": func(r *UpgradeRequest) { r.Action = UpgradeActionRollback },
	} {
		t.Run(name, func(t *testing.T) {
			request := validUpgradeRequest()
			mutate(&request)
			data, _ := json.Marshal(request)
			if _, err := ParseUpgradeRequest(data); !errors.Is(err, ErrInvalidUpgradeRequest) {
				t.Fatalf("ParseUpgradeRequest() = %v, want ErrInvalidUpgradeRequest", err)
			}
		})
	}
	if _, err := ParseUpgradeRequest(make([]byte, MaxUpgradeRequestBytes+1)); !errors.Is(err, ErrInvalidUpgradeRequest) {
		t.Fatalf("oversize payload: %v", err)
	}
}

func TestSameAgentVersion(t *testing.T) {
	if !SameAgentVersion("v4.2.0", "4.2.0") || !SameAgentVersion(" v4.2.0", "v4.2.0") {
		t.Fatal("same release not matched")
	}
	if SameAgentVersion("v4.2.0", "v4.2.1") || SameAgentVersion("", "") {
		t.Fatal("different or empty releases matched")
	}
}
