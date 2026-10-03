package agentcontrol

import (
	"strings"
	"testing"
)

func TestValidatePackageReportKind(t *testing.T) {
	for _, kind := range []string{"systemd.services", "forward", "forward.rules-v2", "a"} {
		if err := ValidatePackageReportKind(kind); err != nil {
			t.Errorf("%q: %v", kind, err)
		}
	}
	for _, kind := range []string{"", "Systemd.services", "systemd..services", ".systemd", "systemd.", "1systemd", "systemd.services/v1", "systemd services", strings.Repeat("a", MaxPackageReportKindLength+1)} {
		if err := ValidatePackageReportKind(kind); err == nil {
			t.Errorf("%q accepted", kind)
		}
	}
	if err := ValidatePackageReportKind(strings.Repeat("a", MaxPackageReportKindLength)); err != nil {
		t.Errorf("kind of the maximal length: %v", err)
	}
}

func TestValidatePackageReportPayloadSize(t *testing.T) {
	if err := ValidatePackageReportPayloadSize(make([]byte, MaxPackageReportPayloadBytes)); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackageReportPayloadSize(make([]byte, MaxPackageReportPayloadBytes+1)); err != ErrPackageReportTooLarge {
		t.Fatalf("oversize payload: %v", err)
	}
}
