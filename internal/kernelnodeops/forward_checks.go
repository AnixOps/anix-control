package kernelnodeops

import (
	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
)

// ForwardChecks answers the kernel's route diagnosis
// (kernelforward.DiagnoseRoute) whether a node's Agent is connected on an
// Agent Control stream and advertises agent.diagnostic and diag.v1.
type ForwardChecks struct {
	// Sources are the live sessions; DefaultAgentSources when nil.
	Sources SourcesFunc
}

var _ kernelforward.NodeChecker = ForwardChecks{}

func (c ForwardChecks) streams() agentstreams.Streams {
	if c.Sources == nil {
		return DefaultAgentSources().Streams
	}
	return c.Sources().Streams
}

// Vantage answers whether node's Agent is connected and runs the checks.
func (c ForwardChecks) Vantage(node agentcontrol.AgentNode) kernelforward.NodeVantage {
	streams := c.streams()
	if streams == nil {
		return kernelforward.NodeVantage{Note: "this Control process holds no Agent Control streams"}
	}
	session, ok := streams.Session(node)
	if !ok {
		return kernelforward.NodeVantage{Note: "the node's Agent is not connected"}
	}
	if !forwardCheckCapable(session.Capabilities) {
		note := "the node's Agent does not advertise agent.diagnostic and diag.v1"
		if session.AgentVersion != "" {
			note += " (Agent " + session.AgentVersion + ")"
		}
		return kernelforward.NodeVantage{Connected: true, Note: note}
	}
	return kernelforward.NodeVantage{Connected: true, Capable: true}
}

func forwardCheckCapable(capabilities []string) bool {
	return hasCapability(capabilities, AgentDiagnosticOperation) && hasCapability(capabilities, agentcontrol.CapabilityDiag)
}
