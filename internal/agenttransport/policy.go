// Package agenttransport carries the agent transport transition of
// node-ops-service.md, section 5.6 (A2-6): the agent_control.mtls policy on
// the legacy AnixOps-agent HTTP and WebSocket paths, the deprecation
// signals, the legacy request counters, and the transport inventory (which
// channel each node's agent was last seen on).
//
// The policy covers the AnixOps Agent channels only. UniProxy and the
// v2board gRPC services, which third-party node software (V2bX, XrayR)
// shares, are recorded in the inventory but never signalled or refused.
package agenttransport

import (
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
)

// UpgradeGuideURL is where the deprecation signals point: the UPGRADE
// section that walks an operator from legacy agents to enrolled ones.
const UpgradeGuideURL = "https://github.com/AnixOps/anix-control/blob/go_dev/docs/UPGRADE.md#agent-transports-preparing-for-v42"

// Policy is the effective agent_control configuration of this process.
type Policy struct {
	// Mode is agent_control.mtls, already defaulted (config.AgentMTLS*).
	Mode string
	// Sunset is agent_control.legacy_sunset; zero sends no Sunset.
	Sunset time.Time
}

// PolicyFrom returns the policy of an agent_control configuration. An
// invalid sunset (which configuration validation refuses) is ignored.
func PolicyFrom(cfg config.AgentControlConfig) Policy {
	sunset, err := cfg.LegacySunsetTime()
	if err != nil {
		sunset = time.Time{}
	}
	return Policy{Mode: cfg.MTLSOrDefault(), Sunset: sunset}
}

// Signals tells whether legacy agents are answered with deprecation
// signals: in preferred mode (and on required's refusals).
func (p Policy) Signals() bool {
	return p.Mode == config.AgentMTLSPreferred || p.Mode == config.AgentMTLSRequired
}

// RefusesLegacy tells whether the legacy AnixOps-agent channels are
// refused (required).
func (p Policy) RefusesLegacy() bool { return p.Mode == config.AgentMTLSRequired }

// SunsetHeader is the HTTP-date of Sunset, empty without one.
func (p Policy) SunsetHeader() string {
	if p.Sunset.IsZero() {
		return ""
	}
	return p.Sunset.UTC().Format(http.TimeFormat)
}

// SetDeprecationHeaders writes the deprecation signals of a legacy agent
// request: Deprecation (draft-ietf-httpapi-deprecation-header, as "true"),
// Sunset (RFC 8594) when a date is configured, and a Link to the upgrade
// guide with rel="deprecation" (and rel="sunset" with a date).
func (p Policy) SetDeprecationHeaders(header http.Header) {
	header.Set("Deprecation", "true")
	rel := "deprecation"
	if sunset := p.SunsetHeader(); sunset != "" {
		header.Set("Sunset", sunset)
		rel = "deprecation sunset"
	}
	header.Add("Link", "<"+UpgradeGuideURL+`>; rel="`+rel+`"; type="text/html"`)
}

// RefusalMessage explains a refused legacy agent request.
const RefusalMessage = "legacy agent authentication is refused (agent_control.mtls: required): enroll the agent with anix.agent.v1.AgentEnrollment and connect on the mTLS Agent Control stream; see " + UpgradeGuideURL
