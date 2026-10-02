package agenttransport

import (
	"net/http"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/agentws"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
)

// LegacyHTTP guards a legacy AnixOps-agent HTTP or WebSocket path
// (/api/v2/agent/*, /api/v2/node/*, /api/v2/forward/agent/rules, the clean
// agent endpoints). It goes before the path's authentication:
//   - required refuses the request with 403 and the code
//     agent_mtls_required, with the deprecation headers;
//   - preferred serves it with the deprecation headers;
//   - optional and off serve it silently.
//
// Every request it serves counts in anixops_agent_legacy_requests_total,
// in every mode, so an operator can watch the legacy traffic drain before
// switching to required. After the handler, an HTTP request the kernel
// authenticated (node_id in the context) is recorded in the transport
// inventory; WebSocket sessions are recorded by their handler, since this
// middleware returns only when the session ends.
func LegacyHTTP(policy Policy, transport string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.FullPath()
		if policy.RefusesLegacy() {
			CountRefused(path)
			policy.SetDeprecationHeaders(c.Writer.Header())
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": agentcontrol.ErrorCodeMTLSRequired, "error": RefusalMessage})
			return
		}
		CountLegacy(path)
		if policy.Signals() {
			policy.SetDeprecationHeaders(c.Writer.Header())
		}
		c.Next()
		if transport != model.AgentTransportWebSocket {
			recordAuthenticated(c, transport)
		}
	}
}

// ThirdPartyHTTP records the nodes a third-party node protocol (UniProxy)
// authenticated in the transport inventory. It never signals or refuses:
// agent_control.mtls does not apply to these protocols.
func ThirdPartyHTTP(transport string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		recordAuthenticated(c, transport)
	}
}

// recordAuthenticated records the node the kernel's authentication put in
// the request context, when the request succeeded.
func recordAuthenticated(c *gin.Context, transport string) {
	if c.Writer.Status() >= http.StatusBadRequest {
		return
	}
	id := contextNodeID(c)
	if id == 0 {
		return
	}
	kind := agentcontrol.NodeKindProxy
	if forward, ok := c.Get(agentws.ForwardNodeContextKey); ok && forward == true {
		kind = agentcontrol.NodeKindForward
	}
	Seen(c.Request.Context(), Sighting{
		Node: agentcontrol.AgentNode{Kind: kind, ID: id}, Transport: transport, Identity: agentstreams.IdentityAPIKey,
	})
}

func contextNodeID(c *gin.Context) uint32 {
	value, ok := c.Get("node_id")
	if !ok {
		return 0
	}
	switch id := value.(type) {
	case uint:
		return uint32(id) // #nosec G115 -- node ids are uint32 on every agent channel.
	case uint32:
		return id
	case uint64:
		return uint32(id) // #nosec G115 -- node ids are uint32 on every agent channel.
	case int:
		if id > 0 {
			return uint32(id) // #nosec G115 -- node ids are uint32 on every agent channel.
		}
	}
	return 0
}
