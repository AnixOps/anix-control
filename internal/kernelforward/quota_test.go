package kernelforward

import (
	"fmt"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/quota"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func TestCreateForwardNodeAsksTheQuotaFirst(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	server := (&Server{Service: f.service, Authorizer: grants{service.CapabilityForward: true}}).For(host)
	var before int64
	require.NoError(t, f.service.DB.Model(&model.ForwardNode{}).Count(&before).Error)
	var asked []quota.Kind
	quota.Check = func(_ *gorm.DB, kind quota.Kind) error {
		asked = append(asked, kind)
		return fmt.Errorf("%w: node limit reached", quota.ErrExceeded)
	}
	t.Cleanup(func() { quota.Check = nil })

	_, err := server.CreateForwardNode(f.ctx, &forwardv1.CreateForwardNodeRequest{RequestId: "q1", Node: &forwardv1.ForwardNodeRecord{
		Name: "n", Host: "192.0.2.9", Transport: forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE,
	}})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, []quota.Kind{quota.Nodes}, asked)
	var after int64
	require.NoError(t, f.service.DB.Model(&model.ForwardNode{}).Count(&after).Error)
	assert.Equal(t, before, after, "a refusal creates nothing and keeps the existing nodes")
}
