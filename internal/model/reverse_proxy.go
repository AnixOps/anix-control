package model

import "time"

// ReverseProxy is a panel HTTP(S) reverse proxy (internal/reverseproxy):
// its entry devices answer the sites' names on ListenPort and send each
// request, by host and path, to the route's upstreams, dialed directly or
// from the last of the Via devices over Control's links. The entries run
// it as a NetworkPlan reverse proxy.
//
// EntriesJSON and ViaJSON are node references (proxy-<id>) as JSON
// arrays; SitesJSON is the sites as the API takes them. TLSMode is none
// (plain HTTP), manual (the administrator's certificate) or acme (issued
// by Control with a DNS-01 challenge through DNSProviderID, a
// KernelForwardDNSProvider, in its zone DNSZone). The private key is sealed under
// module_runtime.ca_kek. HTTPPort, when not 0, is a plain listener that
// redirects every request to https.
type ReverseProxy struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	Name                string     `gorm:"size:100;not null;uniqueIndex" json:"name"`
	Enabled             bool       `gorm:"not null;default:true" json:"enabled"`
	EntriesJSON         string     `gorm:"type:text;not null;default:''" json:"-"`
	ViaJSON             string     `gorm:"type:text;not null;default:''" json:"-"`
	ListenPort          uint32     `gorm:"not null;default:0" json:"listen_port"`
	HTTPPort            uint32     `gorm:"not null;default:0" json:"http_port"`
	SitesJSON           string     `gorm:"type:text;not null;default:''" json:"-"`
	TLSMode             string     `gorm:"size:16;not null;default:'none'" json:"tls_mode"`
	DNSProviderID       uint       `gorm:"not null;default:0" json:"dns_provider_id"`
	DNSZone             string     `gorm:"size:253;not null;default:''" json:"dns_zone"`
	CertificatePEM      string     `gorm:"type:text;not null;default:''" json:"-"`
	SealedPrivateKey    string     `gorm:"type:text;not null;default:''" json:"-"`
	CertificateNotAfter *time.Time `json:"certificate_not_after"`
	CertificateError    string     `gorm:"type:text;not null;default:''" json:"certificate_error"`
	// CertificateAttemptAt is when Control last tried to issue an ACME
	// certificate; after a failure it waits before the next try.
	CertificateAttemptAt *time.Time `json:"certificate_attempt_at"`
	CreatedAt            time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"not null" json:"updated_at"`
}

func (ReverseProxy) TableName() string { return "v5_reverse_proxy" }

// ReverseProxyACMEAccount is Control's ACME account at one directory, for
// the reverse proxies' certificates: its key sealed under
// module_runtime.ca_kek and the account URL the directory answered.
type ReverseProxyACMEAccount struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Directory  string    `gorm:"size:512;not null;uniqueIndex" json:"directory"`
	SealedKey  string    `gorm:"type:text;not null;default:''" json:"-"`
	AccountURL string    `gorm:"size:512;not null;default:''" json:"account_url"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
}

func (ReverseProxyACMEAccount) TableName() string { return "v5_reverse_proxy_acme_account" }
