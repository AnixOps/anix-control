package identitybridge

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestNewAllowlistInvokesTheIdentityLoginAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	config.Set(&config.Config{JWT: config.JWTConfig{Secret: "identity-bridge-test-secret", Expire: 3600}})
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"}))
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, database.Get().AutoMigrate(&model.User{}))

	allowlist, err := NewAllowlist(config.Get())
	require.NoError(t, err)

	metadata, err := json.Marshal(map[string]any{"path": "/api/v2/login"})
	require.NoError(t, err)
	response, err := allowlist.Invoke(context.Background(), packagebridge.Call{
		Host: packagebridge.HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 1},
		Request: packagebridge.Request{
			RequestID: "identity-bridge-login", RouteID: "identity.auth.login", Method: "POST",
			Body: []byte(`{}`), PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"identity-platform"}`),
			MetadataJSON: metadata, Deadline: time.Now().Add(time.Second),
		},
		Operation: "identity.auth.login",
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	var panel map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &panel))
	require.EqualValues(t, -1, panel["code"])
}

func TestNewAllowlistExecutesOnlyTheFixedIdentityMigration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.Set(&config.Config{JWT: config.JWTConfig{Secret: "identity-migration-test-secret", Expire: 3600}})
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"}))
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, database.Get().AutoMigrate(&model.SystemConfig{}))

	allowlist, err := NewAllowlist(config.Get())
	require.NoError(t, err)
	call := packagebridge.Call{
		Host: packagebridge.HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 1},
		Request: packagebridge.Request{
			RequestID: "identity-migration-1", RouteID: "migration.identity-platform.001_identity_platform", Method: "MIGRATE",
			Deadline: time.Now().Add(time.Second),
		},
		Operation: "migration.identity-platform.001_identity_platform",
	}

	response, err := allowlist.Invoke(context.Background(), call)

	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.True(t, database.Get().Migrator().HasTable("identity_platform_projection"))
	var checkpoint struct {
		Checkpoint       string `json:"checkpoint"`
		ValidationDigest string `json:"validation_digest"`
		Complete         bool   `json:"complete"`
	}
	require.NoError(t, json.Unmarshal(response.Body, &checkpoint))
	require.Equal(t, "identity-platform/001", checkpoint.Checkpoint)
	require.NotEmpty(t, checkpoint.ValidationDigest)
	require.True(t, checkpoint.Complete)

	call.Payload = []byte("DROP TABLE users")
	_, err = allowlist.Invoke(context.Background(), call)
	require.ErrorIs(t, err, packagebridge.ErrCapabilityRejected)
}

func TestNewFactoryUsesTheRegisteredWebSocketRouteResolver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "identity-websocket-bridge-test-secret", Expire: 3600}}
	config.Set(cfg)
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"}))
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, database.Get().AutoMigrate(&model.User{}))

	const routeID = "identity.factory.websocket.test"
	packagebridge.DefaultRouteRegistry().MustRegisterWebSocket("identity-platform", routeID, func(c *gin.Context) {
		connection, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_, data, err := connection.ReadMessage()
		if err == nil {
			_ = connection.WriteMessage(websocket.TextMessage, append([]byte("echo:"), data...))
		}
	})

	factory, err := NewFactory(cfg)
	require.NoError(t, err)
	session, child, err := factory.NewSession(packagebridge.HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 7})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	capability, err := session.Mint(packagebridge.Request{
		RequestID: "identity-factory-websocket-1", RouteID: routeID, Method: http.MethodGet,
		PrincipalJSON: []byte(`{"actor_id":1,"admin":true,"package_id":"identity-platform"}`),
		MetadataJSON:  []byte(`{"path":"/api/v2/login"}`), Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)

	stream, err := client.OpenWebSocket(context.Background(), capability, routeID)
	require.NoError(t, err)
	require.NoError(t, stream.Send(packagebridgesdk.WebSocketFrame{Data: []byte("ping")}))
	frame, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, []byte("echo:ping"), frame.Data)
}
