package agentcontrol

import (
	"testing"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
)

func TestDataPlaneCapabilityNames(t *testing.T) {
	for name, want := range map[string]string{
		CapabilityConfig:  "config",
		CapabilityUsers:   "users",
		CapabilityReports: "reports",
	} {
		if name != want {
			t.Fatalf("capability = %q, want %q", name, want)
		}
	}
}

func TestNegotiatedRequiresBothSidesAtV1(t *testing.T) {
	reportsV1 := []*agentv1pb.Capability{{Name: CapabilityReports, Version: CapabilityVersionV1}}
	tests := []struct {
		name          string
		agent, server []*agentv1pb.Capability
		want          bool
	}{
		{name: "both", agent: reportsV1, server: reportsV1, want: true},
		{name: "agent only", agent: reportsV1},
		{name: "server only, as to an older Agent", server: reportsV1},
		{name: "neither"},
		{name: "other version", agent: reportsV1, server: []*agentv1pb.Capability{{Name: CapabilityReports, Version: "v2"}}},
		{name: "other feature", agent: reportsV1, server: []*agentv1pb.Capability{nil, {Name: CapabilityConfig, Version: CapabilityVersionV1}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Negotiated(test.agent, test.server, CapabilityReports); got != test.want {
				t.Fatalf("Negotiated() = %v, want %v", got, test.want)
			}
		})
	}
}
