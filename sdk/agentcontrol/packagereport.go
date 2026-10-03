package agentcontrol

import (
	"errors"
	"fmt"
	"regexp"
)

// Generic limits of a PackageReport (package-reports.v1; PROTOCOL.md,
// "Package reports"). They hold for every kind; each kind's schema adds its
// own rules (for systemd.services, sdk/telemetry/systemdreport).
const (
	// MaxPackageReportPayloadBytes caps PackageReport.payload_json.
	MaxPackageReportPayloadBytes = 256 << 10
	// MaxPackageReportKindLength caps PackageReport.kind.
	MaxPackageReportKindLength = 64
)

// packageReportKindPattern: lowercase dot-separated words, each starting
// with a letter, such as systemd.services.
var packageReportKindPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)*$`)

var (
	// ErrPackageReportKind means PackageReport.kind is empty, too long or
	// not lowercase dot-separated words.
	ErrPackageReportKind = errors.New("package report kind is invalid")
	// ErrPackageReportTooLarge means payload_json exceeds
	// MaxPackageReportPayloadBytes.
	ErrPackageReportTooLarge = fmt.Errorf("package report payload exceeds %d bytes", MaxPackageReportPayloadBytes)
)

// ValidatePackageReportKind checks a PackageReport kind.
func ValidatePackageReportKind(kind string) error {
	if len(kind) == 0 || len(kind) > MaxPackageReportKindLength || !packageReportKindPattern.MatchString(kind) {
		return ErrPackageReportKind
	}
	return nil
}

// ValidatePackageReportPayloadSize checks payload_json against
// MaxPackageReportPayloadBytes.
func ValidatePackageReportPayloadSize(payload []byte) error {
	if len(payload) > MaxPackageReportPayloadBytes {
		return ErrPackageReportTooLarge
	}
	return nil
}
