package handler

import (
	"fmt"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/quota"
	"gorm.io/gorm"
)

func (s *NodeCredentialAnswersTestSuite) TestCreateNodeAsksTheQuotaFirst() {
	quota.Check = func(_ *gorm.DB, kind quota.Kind) error {
		return fmt.Errorf("%w: %s", quota.ErrExceeded, kind)
	}
	defer func() { quota.Check = nil }()

	code, _ := s.serve(http.MethodPost, "/admin/nodes", "/admin/nodes", `{"name":"edge","host":"edge.example.test","port":443}`, NewNodeHandler().CreateNode)
	s.Equal(http.StatusForbidden, code)
	s.Equal(int64(0), s.count(&model.Node{}, "1 = 1"))
	s.Equal(int64(0), s.count(&model.NodeProtocol{}, "1 = 1"))
}
