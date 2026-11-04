package agentcontrol

import "testing"

func TestHelloProtocolRevision(t *testing.T) {
	cases := map[string]uint32{"": ProtocolRevisionBaseline, "1": 1, "7": 7, " 3 ": 3, "0": ProtocolRevisionBaseline, "-2": ProtocolRevisionBaseline, "x": ProtocolRevisionBaseline, "9999999999": ProtocolRevisionBaseline}
	for value, want := range cases {
		labels := map[string]string{}
		if value != "" {
			labels[LabelProtocolRevision] = value
		}
		if got := HelloProtocolRevision(labels); got != want {
			t.Errorf("HelloProtocolRevision(%q) = %d, want %d", value, got, want)
		}
	}
	if got := HelloProtocolRevision(nil); got != ProtocolRevisionBaseline {
		t.Errorf("HelloProtocolRevision(nil) = %d", got)
	}
}

func TestHelloKernelMinVersion(t *testing.T) {
	if got := HelloKernelMinVersion(map[string]string{LabelKernelMinVersion: "0.1.0-alpha.15"}); got != "0.1.0-alpha.15" {
		t.Fatalf("got %q", got)
	}
	if got := HelloKernelMinVersion(map[string]string{LabelKernelMinVersion: "v0.1.0"}); got != "" {
		t.Fatalf("malformed accepted: %q", got)
	}
}

func TestVersionLine(t *testing.T) {
	for version, want := range map[string]string{"v5.0.0": "5.0", "5.0.12": "5.0", "v5.1.0-rc.2": "5.1", "v5.0.0-rc.25.hf.1": "5.0", "v10.20.3-dev": "10.20"} {
		if got, ok := VersionLine(version); !ok || got != want {
			t.Errorf("VersionLine(%q) = %q, %v; want %q", version, got, ok, want)
		}
	}
	for _, version := range []string{"", "5.0", "dev", "v5"} {
		if _, ok := VersionLine(version); ok {
			t.Errorf("VersionLine(%q) accepted", version)
		}
	}
	if !SameVersionLine("v5.0.3", "v5.0.0") || SameVersionLine("v5.1.0", "v5.0.9") || SameVersionLine("v5.0.0", "") {
		t.Fatal("SameVersionLine")
	}
}

func TestCompareKernelVersions(t *testing.T) {
	ordered := []string{"0.1.0-alpha.9", "0.1.0-alpha.15", "0.1.0-beta.1", "0.1.0-rc.1", "0.1.0", "0.1.1", "0.2.0", "1.0.0"}
	for i := range ordered {
		for j := range ordered {
			got, ok := CompareKernelVersions(ordered[i], ordered[j])
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if !ok || got != want {
				t.Errorf("CompareKernelVersions(%q, %q) = %d, %v; want %d", ordered[i], ordered[j], got, ok, want)
			}
		}
	}
	if _, ok := CompareKernelVersions("0.1.0", "latest"); ok {
		t.Fatal("malformed compared")
	}
}

func TestCheckCompatibility(t *testing.T) {
	base := CompatibilityInput{ControlVersion: "v5.0.2", MinRevision: 2, MaxRevision: 3, AgentVersion: "v5.0.7", AgentRevision: 2, KernelVersion: "0.1.0-alpha.15", KernelMinVersion: "0.1.0-alpha.15"}
	cases := []struct {
		name   string
		modify func(*CompatibilityInput)
		status string
		codes  []string
	}{
		{"ok", func(*CompatibilityInput) {}, CompatibilityOK, nil},
		{"patch numbers are independent", func(in *CompatibilityInput) { in.AgentVersion = "v5.0.0" }, CompatibilityOK, nil},
		{"old protocol", func(in *CompatibilityInput) { in.AgentRevision = 1 }, CompatibilityIncompatible, []string{ProblemAgentProtocolTooOld}},
		{"no label is the baseline", func(in *CompatibilityInput) { in.AgentRevision = 0 }, CompatibilityIncompatible, []string{ProblemAgentProtocolTooOld}},
		{"new protocol", func(in *CompatibilityInput) { in.AgentRevision = 4 }, CompatibilityWarning, []string{ProblemAgentProtocolTooNew}},
		{"other line", func(in *CompatibilityInput) { in.AgentVersion = "v5.1.0" }, CompatibilityWarning, []string{ProblemVersionLineMismatch}},
		{"unparsable versions skip the line check", func(in *CompatibilityInput) { in.AgentVersion = "dev" }, CompatibilityOK, nil},
		{"old kernel", func(in *CompatibilityInput) { in.KernelVersion = "0.1.0-alpha.13" }, CompatibilityIncompatible, []string{ProblemKernelTooOld}},
		{"no kernel minimum", func(in *CompatibilityInput) { in.KernelVersion, in.KernelMinVersion = "0.1.0-alpha.1", "" }, CompatibilityOK, nil},
		{"warning then incompatible", func(in *CompatibilityInput) {
			in.AgentVersion = "v5.1.0"
			in.KernelVersion = "0.1.0-alpha.1"
		}, CompatibilityIncompatible, []string{ProblemVersionLineMismatch, ProblemKernelTooOld}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.modify(&in)
			got := CheckCompatibility(in)
			if got.Status != tc.status || len(got.Problems) != len(tc.codes) {
				t.Fatalf("got %+v, want %s %v", got, tc.status, tc.codes)
			}
			for i, code := range tc.codes {
				if got.Problems[i].Code != code || got.Problems[i].Message == "" {
					t.Fatalf("problem %d = %+v, want %s", i, got.Problems[i], code)
				}
			}
		})
	}
}
