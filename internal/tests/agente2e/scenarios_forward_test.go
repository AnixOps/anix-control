package agente2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// scenarioLinkCertificate: once forward.v1 is negotiated the Agent asks
// Control's link CA for its forward link certificate (H28) over the agent
// certificate's mTLS. Since gost was removed only the anixops relay asks for
// one, and the suite does not run the relay (Forward.AnixOps.Enable is off),
// so the scenario is skipped; Control's IssueLinkCertificate keeps its unit
// tests.
func (s *suite) scenarioLinkCertificate(t *testing.T) string {
	return skipped + ":the Agent asks for a link certificate only for the anixops relay, which the suite does not run; gost was removed"
}

type forwardReport struct {
	Generation uint64
	StateHash  string
	Applied    bool
	HopErrors  int
	ReportJSON string
	ObservedAt time.Time
	ReceivedAt time.Time
}

func (s *suite) forwardNodeRef() string { return fmt.Sprintf("proxy-%d", s.nodeID) }

func (s *suite) forwardReport(t *testing.T) forwardReport {
	t.Helper()
	var report forwardReport
	require.NoError(t, s.control.db.Raw("SELECT generation, state_hash, applied, hop_errors, report_json, observed_at, received_at FROM v4_kernel_forward_node_report WHERE node_ref = ?", s.forwardNodeRef()).Scan(&report).Error)
	return report
}
