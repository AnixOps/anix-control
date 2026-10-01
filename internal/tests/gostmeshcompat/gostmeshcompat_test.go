// Package gostmeshcompat proves the gost-mesh package's native gost API
// connection test answers exactly as the kernel's legacy handler, on SQLite
// and PostgreSQL (it reads no table; both backends run for uniformity). Both
// sides call the same test gost APIs: one that answers its service list to
// the right token, one that refuses, fails or answers what the client cannot
// decode, and an address nothing listens on.
package gostmeshcompat

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/gost-mesh/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	path    = "/api/v2/admin/forward/test-connection"
	routeID = "gost.admin.forward.test_connection.post"
	token   = "gost-api-token"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

func testRoute() packagecompat.Route {
	return packagecompat.Route{
		Method: "POST", Pattern: path, RouteID: routeID,
		Legacy: func(c *gin.Context) { handler.NewForwardHandler().TestGostConnection(c) },
		Native: func(*gorm.DB) pluginhostsdk.NativeHandler { return (&native.Service{}).Handlers()[routeID] },
	}
}

// gostAPI serves answer for GET /api/config/services when the request
// carries the token as its basic auth password, else 401.
func gostAPI(t *testing.T, answer func(w http.ResponseWriter, call int64)) (host string, port int) {
	t.Helper()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/config/services" {
			http.NotFound(w, r)
			return
		}
		if _, password, _ := r.BasicAuth(); password != token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"msg":"unauthorized"}`))
			return
		}
		answer(w, calls.Add(1))
	}))
	t.Cleanup(server.Close)
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err = strconv.Atoi(address.Port())
	require.NoError(t, err)
	return address.Hostname(), port
}

func body(text string) func(http.ResponseWriter, int64) {
	return func(w http.ResponseWriter, _ int64) { _, _ = w.Write([]byte(text)) }
}

// closedPort is a local port nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func request(host string, port int, apiToken string) []byte {
	return []byte(fmt.Sprintf(`{"host":%q,"api_port":%d,"api_token":%q}`, host, port, apiToken))
}

func TestGostConnectionTestRouteParity(t *testing.T) {
	healthy, healthyPort := gostAPI(t, body(`{"data":{"count":2,"list":[{"name":"svc-a","addr":":8080","handler":{"type":"tcp"}},{"name":"svc-b","addr":":8081"}]}}`))
	_, emptyPort := gostAPI(t, body(`{}`))
	_, failingPort := gostAPI(t, func(w http.ResponseWriter, _ int64) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream <down> & out"))
	})
	_, notJSONPort := gostAPI(t, body(`<html>gost</html>`))
	_, wrongCountPort := gostAPI(t, body(`{"data":{"count":"two"}}`))
	_, wrongNestedPort := gostAPI(t, body(`{"data":{"count":1,"list":[{"name":"svc","handler":{"type":7}}]}}`))
	// The health check passes and the service list then fails: odd calls
	// answer, even calls fail, and each side makes two calls.
	_, flakyPort := gostAPI(t, func(w http.ResponseWriter, call int64) {
		if call%2 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("services unavailable"))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"count":1,"list":[]}}`))
	})
	refused := closedPort(t)

	cases := []packagecompat.Case{
		{Name: "a healthy gost API", Body: request(healthy, healthyPort, token)},
		{Name: "an empty service list", Body: request(healthy, emptyPort, token)},
		{Name: "a wrong token", Body: request(healthy, healthyPort, "wrong")},
		{Name: "no token", Body: request(healthy, healthyPort, "")},
		{Name: "an API error", Body: request(healthy, failingPort, token)},
		{Name: "an answer that is not JSON", Body: request(healthy, notJSONPort, token)},
		{Name: "a count that is not a number", Body: request(healthy, wrongCountPort, token)},
		{Name: "a nested field of the wrong type", Body: request(healthy, wrongNestedPort, token)},
		{Name: "the service list fails after the health check", Body: request(healthy, flakyPort, token)},
		{Name: "nothing listens", Body: request("127.0.0.1", refused, token)},
		{Name: "a host that is not a host name", Body: request("gost host", healthyPort, token)},
		{Name: "a negative port", Body: request(healthy, -1, token)},
		{Name: "a host with a path", Body: request(healthy+":"+strconv.Itoa(healthyPort)+"/x?", healthyPort, token)},
		{Name: "no body"},
		{Name: "invalid JSON", Body: []byte(`{"host":`)},
		{Name: "no host", Body: []byte(`{"api_port":8080}`)},
		{Name: "no port", Body: []byte(`{"host":"127.0.0.1"}`)},
		{Name: "port zero", Body: []byte(`{"host":"127.0.0.1","api_port":0}`)},
		{Name: "a port that is a string", Body: []byte(`{"host":"127.0.0.1","api_port":"8080"}`)},
		{Name: "a host that is a number", Body: []byte(`{"host":7,"api_port":8080}`)},
		{Name: "a body that is an array", Body: []byte(`[1]`)},
		{Name: "a body that is a string", Body: []byte(`"127.0.0.1"`)},
		{Name: "a body that is null", Body: []byte(`null`)},
	}
	for _, c := range cases {
		c.Path, c.Principal = path, admin
		packagecompat.RunRead(t, testRoute(), c)
	}
}

// The native client sends the token as the gost API's basic auth password
// and reaches the API, so the cases compare real answers.
func TestNativeConnectionTestReachesTheAPI(t *testing.T) {
	host, port := gostAPI(t, body(`{"data":{"count":3,"list":[]}}`))
	response, err := (&native.Service{}).TestConnection(context.Background(), pluginhostsdk.NativeRequest{Body: request(host, port, token)})
	require.NoError(t, err)
	require.Contains(t, string(response.Body), `"data":{"message":"Connection successful","service_count":3,"success":true}`)
}
