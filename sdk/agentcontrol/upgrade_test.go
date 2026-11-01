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
	// Dev builds and hotfixes are release tags too.
	for _, tag := range []string{"v5.0.0-rc.25.dev.3", "v5.0.0-rc.25.hf.1"} {
		hotfix := validUpgradeRequest()
		hotfix.TargetVersion, hotfix.PreviousVersion = tag, "v5.0.0-rc.25"
		if err := hotfix.Validate(); err != nil {
			t.Fatalf("%s refused: %v", tag, err)
		}
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

func validKernel() UpgradeKernel {
	asset := "anx-node-0.1.0-alpha.2-x86_64-linux-musl.tar.gz"
	digest := strings.Repeat("b", 64)
	return UpgradeKernel{
		Name: KernelAnxNode, Version: "0.1.0-alpha.2", Arch: "amd64", Asset: asset,
		URL: "https://control.example/install/kernel/anx-node/0.1.0-alpha.2/" + asset, SHA256: digest, Size: 2048,
		Sums:          digest + "  " + asset + "\n" + strings.Repeat("c", 64) + "  anx-node\n",
		SumsSignature: "-----BEGIN SSH SIGNATURE-----\nU1NIU0lH\n-----END SSH SIGNATURE-----\n",
	}
}

func TestUpgradeRequestKernels(t *testing.T) {
	request := validUpgradeRequest()
	request.Kernels = []UpgradeKernel{validKernel()}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if kernel, ok := request.Kernel("amd64"); !ok || kernel.Size != 2048 {
		t.Fatalf("Kernel(amd64) = %+v, %v", kernel, ok)
	}
	if _, ok := request.Kernel("arm64"); ok {
		t.Fatal("Kernel(arm64) found")
	}
	for name, mutate := range map[string]func(*UpgradeKernel){
		"name":          func(k *UpgradeKernel) { k.Name = "sing-box" },
		"version":       func(k *UpgradeKernel) { k.Version = "v0.1.0" },
		"asset":         func(k *UpgradeKernel) { k.Asset = "../anx-node" },
		"other version": func(k *UpgradeKernel) { k.Version = "0.1.0-alpha.3" },
		"http url":      func(k *UpgradeKernel) { k.URL = "http://control.example/k.tar.gz" },
		"size":          func(k *UpgradeKernel) { k.Size = MaxUpgradeKernelBytes + 1 },
		"unlisted":      func(k *UpgradeKernel) { k.SHA256 = strings.Repeat("d", 64) },
		"no sums":       func(k *UpgradeKernel) { k.Sums = "" },
		"signature":     func(k *UpgradeKernel) { k.SumsSignature = "c2ln" },
	} {
		t.Run(name, func(t *testing.T) {
			request := validUpgradeRequest()
			kernel := validKernel()
			mutate(&kernel)
			request.Kernels = []UpgradeKernel{kernel}
			if err := request.Validate(); !errors.Is(err, ErrInvalidUpgradeRequest) {
				t.Fatalf("Validate() = %v, want ErrInvalidUpgradeRequest", err)
			}
		})
	}
	request.Kernels = append(request.Kernels, validKernel())
	if err := request.Validate(); !errors.Is(err, ErrInvalidUpgradeRequest) {
		t.Fatalf("repeated kernel architecture: %v", err)
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
