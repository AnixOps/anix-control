package agenttransport

import (
	"net/http"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/agentws"
	"github.com/gin-gonic/gin"
)

// ThirdPartyHTTP records the nodes a third-party node protocol (UniProxy)
// authenticated in the transport inventory. It never refuses: agent
// certificates do not apply to these protocols.
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
