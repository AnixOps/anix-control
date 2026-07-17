// Package branding contains product names shared by the control-plane runtime.
package branding

const (
	ProductName       = "AnixOps"
	ControlName       = "AnixOps Control"
	AgentName         = "AnixOps Agent"
	ControlBinaryName = "anix-control"
	AgentBinaryName   = "anix-agent"
	DefaultVersion    = "4.0.0-alpha.3"

	ControlRepositoryURL = "https://github.com/AnixOps/anix-control"
	AgentRepositoryURL   = "https://github.com/AnixOps/anix-agent"

	// Legacy names remain protocol and upgrade compatibility identifiers.
	LegacyControlName = "V2Board"
	LegacyAgentName   = "V2bX"
)
