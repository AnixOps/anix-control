package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/suite"
)

// ForwardOperationsAnswersTestSuite pins the bytes the legacy forward
// operation routes answer, and what they send NodeX: panel forward changes,
// a tunnel update, the backend sync, the tunnel permission removal and the
// legacy rules. The KernelNodeOps forward executors (NO-7) share these
// routes' implementation; the answers stay what they were before the routes
// moved onto it. Only the values that change from one call to the next are
// normalized (normalizeOperationAnswer).
type ForwardOperationsAnswersTestSuite struct {
	HandlerTestSuite
	nodex *fakeNodeX
}

func TestForwardOperationsAnswers(t *testing.T) {
	suite.Run(t, new(ForwardOperationsAnswersTestSuite))
}

// fakeNodeX is the NodeX control plane: it records every execute request
// and answers success, except for a forward or rule named "failing", which
// it refuses.
type fakeNodeX struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []string
	auth   []string
}

func newFakeNodeX(t *testing.T) *fakeNodeX {
	t.Helper()
	n := &fakeNodeX{}
	n.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n.mu.Lock()
		n.bodies = append(n.bodies, string(body))
		n.auth = append(n.auth, r.Header.Get("Authorization"))
		n.mu.Unlock()
		if r.URL.Path != "/api/v2/internal/forward/runtime/execute" {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(string(body), `"name":"failing"`) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"nodex refused the change"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"backend":"gost","status":2,"message":"nodex applied","result":"applied"}}`))
	}))
	t.Cleanup(n.server.Close)
	return n
}

func (s *ForwardOperationsAnswersTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.nodex = newFakeNodeX(s.T())
	configs := service.NewSystemConfigService(s.db)
	for key, value := range map[string]string{
		"forward.runtime_backend":        model.ForwardRuntimeBackendGost,
		"forward.runtime.nodex.base_url": s.nodex.server.URL,
		"forward.runtime.nodex.token":    "nodex-shared-token",
	} {
		s.Require().NoError(configs.Set(key, value, "string", "forward", "forward operations answers"))
	}
}
