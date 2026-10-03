package agentcontrol

import (
	"errors"
	"regexp"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
)

func TestTransientReportAcksNeedBothSides(t *testing.T) {
	asked := []*agentv1pb.Capability{{Name: CapabilityReports, Version: CapabilityVersionV1, Attributes: map[string]string{ReportsAttributeTransientAck: ReportsTransientAckV1}}}
	plain := []*agentv1pb.Capability{{Name: CapabilityReports, Version: CapabilityVersionV1}}
	other := []*agentv1pb.Capability{{Name: CapabilityReports, Version: CapabilityVersionV1, Attributes: map[string]string{ReportsAttributeTransientAck: "v2"}}}
	wrongName := []*agentv1pb.Capability{nil, {Name: CapabilityMaintenance, Version: CapabilityVersionV1, Attributes: map[string]string{ReportsAttributeTransientAck: ReportsTransientAckV1}}}
	if !TransientReportAcks(asked, asked) {
		t.Fatal("both sides carry the attribute")
	}
	for name, pair := range map[string][2][]*agentv1pb.Capability{
		"server did not echo": {asked, plain},
		"agent did not ask":   {plain, asked},
		"another version":     {other, other},
		"another capability":  {wrongName, wrongName},
	} {
		if TransientReportAcks(pair[0], pair[1]) {
			t.Errorf("%s: transient acks negotiated", name)
		}
	}
}

func TestAckErrorCodesAreWellFormed(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	codes := append([]string{
		ErrorCodeCapabilityNotNegotiated,
		ReportErrorCodeBatchIDInvalid, ReportErrorCodeInvalid, ReportErrorCodeCounterOverflow, ReportErrorCodeNodeGone, ReportErrorCodeUnavailable,
		MaintenanceErrorCodeSchemaUnsupported, MaintenanceErrorCodeBatchTooLarge, MaintenanceErrorCodeEventInvalid,
		MaintenanceErrorCodeWrongNode, MaintenanceErrorCodeNodeGone, MaintenanceErrorCodeUnavailable,
		ErrorCodePluginReleaseAddressInvalid, ErrorCodePluginReleaseAddressMismatch, ErrorCodePluginReleaseNotAssigned,
		ErrorCodePluginReleaseNotFound, ErrorCodePluginReleaseIntegrityFailed, ErrorCodePluginReleaseBusy,
	}, ConfigErrorCodes...)
	seen := map[string]bool{}
	for _, code := range codes {
		if !pattern.MatchString(code) {
			t.Errorf("%q is not an error code", code)
		}
		if seen[code] {
			t.Errorf("%q is defined twice", code)
		}
		seen[code] = true
	}
}

func TestMaintenanceEventOfAnotherNodeIsWrongNode(t *testing.T) {
	now := time.Now()
	event := validMaintenanceEvent(now)
	_, err := ParseMaintenanceEvent(encodeMaintenanceEvent(t, event), AgentNode{Kind: NodeKindProxy, ID: 999}, now)
	if !errors.Is(err, ErrInvalidMaintenanceEvent) || !errors.Is(err, ErrMaintenanceEventWrongNode) {
		t.Fatalf("an event of another node: %v", err)
	}
	event.SchemaVersion = 2
	_, err = ParseMaintenanceEvent(encodeMaintenanceEvent(t, event), AgentNode{Kind: NodeKindProxy, ID: 999}, now)
	if !errors.Is(err, ErrInvalidMaintenanceEvent) || errors.Is(err, ErrMaintenanceEventWrongNode) {
		t.Fatalf("an invalid event is not a wrong node: %v", err)
	}
}
