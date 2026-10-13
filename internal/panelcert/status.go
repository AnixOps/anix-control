package panelcert

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"golang.org/x/crypto/acme/autocert"
	"gorm.io/gorm"
)

// States of a certificate, and of the panel as a whole (its worst
// certificate).
const (
	// StateNotConfigured: Control manages no certificate (tls.mode external
	// without a gRPC name: TLS is someone else's, the gRPC listener is
	// plain).
	StateNotConfigured = "not_configured"
	// StatePending: no certificate yet, the first request is under way.
	StatePending = "pending"
	// StateValid: a certificate from ACME or the files is served and has
	// not ended.
	StateValid = "valid"
	// StateFailed: no usable certificate: the request or the files failed,
	// and the self-signed fallback is served.
	StateFailed = "failed"
)

// Sources of a certificate.
const (
	SourceACME  = "acme"
	SourceFiles = "files"
)

// What a listener serves for a name.
const (
	ServingACME       = "acme"
	ServingFile       = "file"
	ServingSelfSigned = "self_signed"
)

// Roles of a certificate.
const (
	RoleWeb  = "web"
	RoleGRPC = "grpc"
)

// Problem codes: the cause of a failure, for a fix in the console's words.
const (
	ProblemDNSUnresolved  = "dns_unresolved"
	ProblemDNSElsewhere   = "dns_elsewhere"
	ProblemUnreachable    = "unreachable"
	ProblemChallenge      = "challenge_failed"
	ProblemRateLimited    = "rate_limited"
	ProblemCAUnreachable  = "ca_unreachable"
	ProblemCAError        = "ca_error"
	ProblemFileUnreadable = "file_unreadable"
	ProblemExpired        = "expired"
	ProblemNameMismatch   = "name_mismatch"
	ProblemUntrusted      = "untrusted"
)

// CertificateStatus is what Control knows about one certificate it serves.
// It holds no secret.
type CertificateStatus struct {
	Name string `json:"name"`
	// Names are the names the served certificate covers (DNS names and
	// addresses).
	Names  []string `json:"names"`
	Roles  []string `json:"roles"`
	Source string   `json:"source"`
	State  string   `json:"state"`
	// Serving is what handshakes get now: acme, file or self_signed.
	Serving string `json:"serving"`
	// PubliclyTrusted tells whether the served certificate verifies for
	// Name against the system roots, as an Agent and a browser check it.
	PubliclyTrusted bool       `json:"publicly_trusted"`
	Issuer          string     `json:"issuer,omitempty"`
	NotBefore       *time.Time `json:"not_before,omitempty"`
	NotAfter        *time.Time `json:"not_after,omitempty"`
	// RenewAt is when an ACME certificate is renewed (a third of its
	// lifetime before its end, at most 30 days).
	RenewAt *time.Time `json:"renew_at,omitempty"`
	// RenewalFailing is set while a served certificate is past RenewAt and
	// the latest attempt to renew it failed.
	RenewalFailing bool `json:"renewal_failing"`
	// Attempting is set while a request to the CA for it runs.
	Attempting    bool       `json:"attempting"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	// LastError is the latest failure, Problem its class and Fix what to
	// change, in English (the console has its own words per Problem).
	LastError string `json:"last_error,omitempty"`
	Problem   string `json:"problem,omitempty"`
	Fix       string `json:"fix,omitempty"`
	// Files are the certificate files of a files source.
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
}

// ListenerStatus is a listener the certificate layout asked for that
// could not bind: an ACME challenge path that is not Control's.
type ListenerStatus struct {
	Address string `json:"address"`
	Purpose string `json:"purpose"`
	Error   string `json:"error"`
}

// Status is the panel's certificate status: GET /api/v5/panel/status,
// `anix-control tls status`, the banner and the alerts read it.
type Status struct {
	// Mode is tls.mode (acme, files, external).
	Mode string `json:"mode"`
	// State is the worst state of the certificates, not_configured when
	// there is none.
	State    string `json:"state"`
	Domain   string `json:"domain,omitempty"`
	GRPCName string `json:"grpc_name,omitempty"`
	// Directory is the ACME directory in use when it is not the public
	// CA's (a staging or test CA).
	Directory    string              `json:"directory,omitempty"`
	Persistent   bool                `json:"persistent"`
	Certificates []CertificateStatus `json:"certificates"`
	Listeners    []ListenerStatus    `json:"listeners,omitempty"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// stateRank orders states from best to worst.
func stateRank(state string) int {
	switch state {
	case StateValid:
		return 1
	case StatePending:
		return 2
	case StateFailed:
		return 3
	}
	return 0
}

// overall is the worst state of certificates.
func overall(certificates []CertificateStatus) string {
	if len(certificates) == 0 {
		return StateNotConfigured
	}
	worst := StateValid
	for _, certificate := range certificates {
		if stateRank(certificate.State) > stateRank(worst) {
			worst = certificate.State
		}
	}
	return worst
}

// Certificate returns the status of the certificate serving role, if any.
func (s Status) Certificate(role string) (CertificateStatus, bool) {
	for _, certificate := range s.Certificates {
		for _, r := range certificate.Roles {
			if r == role {
				return certificate, true
			}
		}
	}
	return CertificateStatus{}, false
}

// ErrNoStatus: no Control has published a status in this database.
var ErrNoStatus = errors.New("no panel certificate status: Control has not run with this database yet")

// LoadStatus reads the status the running Control last published in db.
func LoadStatus(ctx context.Context, db *gorm.DB) (Status, error) {
	store, err := NewStore(db, nil, nil)
	if err != nil {
		return Status{}, err
	}
	data, err := store.Get(ctx, statusKey)
	if errors.Is(err, autocert.ErrCacheMiss) {
		return Status{}, ErrNoStatus
	}
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

// RequestRetry asks the running Control to try the certificates that are
// failing again (`anix-control tls retry`). It reads the request within its
// next check (seconds).
func RequestRetry(ctx context.Context, db *gorm.DB, now time.Time) error {
	store, err := NewStore(db, nil, nil)
	if err != nil {
		return err
	}
	return store.Put(ctx, retryKey, []byte(now.UTC().Format(time.RFC3339Nano)))
}

// RetryPending tells whether a retry request still waits for the running
// Control to read it.
func RetryPending(ctx context.Context, db *gorm.DB) (bool, error) {
	store, err := NewStore(db, nil, nil)
	if err != nil {
		return false, err
	}
	_, err = store.Get(ctx, retryKey)
	if errors.Is(err, autocert.ErrCacheMiss) {
		return false, nil
	}
	return err == nil, err
}
