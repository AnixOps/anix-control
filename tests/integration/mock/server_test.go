package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)
	assert.NotNil(t, srv)
	assert.Equal(t, port, srv.port)
}

func TestServerLifecycle(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)

	ctx := context.Background()

	// 启动
	err = srv.Start(ctx)
	require.NoError(t, err)

	// 检查 URL
	assert.Contains(t, srv.URL(), "127.0.0.1")

	// 健康检查
	resp, err := http.Get(srv.URL() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "ok", result["status"])

	// 停止
	err = srv.Stop(ctx)
	require.NoError(t, err)
}

func TestUserManagement(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)

	// 添加用户
	user := &MockUser{
		ID:             1,
		UUID:           "test-uuid-1234",
		Email:          "test@example.com",
		SpeedLimit:     100000000,
		DeviceLimit:    5,
		TransferEnable: 10737418240,
	}
	srv.AddUser(user)

	// 验证用户已添加
	assert.Contains(t, srv.users, user.UUID)

	// 获取统计
	stats := srv.GetStats(user.UUID)
	assert.NotNil(t, stats)

	// 移除用户
	srv.RemoveUser(user.UUID)
	assert.NotContains(t, srv.users, user.UUID)
}

func TestNodeConfig(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)
	ctx := context.Background()

	err = srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 无 token 应该返回 401
	resp, err := http.Get(srv.URL() + "/api/v2/server/UniProxy/config")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// 有 token 应该返回配置
	resp, err = http.Get(srv.URL() + "/api/v2/server/UniProxy/config?token=test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var config NodeConfig
	json.NewDecoder(resp.Body).Decode(&config)
	assert.NotEmpty(t, config.Type)
}

func TestGetUsers(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)
	ctx := context.Background()

	// 添加测试用户
	srv.AddUser(&MockUser{ID: 1, UUID: "uuid-1", Email: "user1@example.com"})
	srv.AddUser(&MockUser{ID: 2, UUID: "uuid-2", Email: "user2@example.com"})

	err = srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 获取用户列表
	resp, err := http.Get(srv.URL() + "/api/v2/server/UniProxy/user?node_id=1&token=test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Users []map[string]interface{} `json:"users"`
		Total int                      `json:"total"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Equal(t, 2, result.Total)
	assert.Len(t, result.Users, 2)
}

func TestPushTraffic(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)
	ctx := context.Background()

	uuid := "test-uuid-traffic"
	srv.AddUser(&MockUser{ID: 1, UUID: uuid})

	err = srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 上报流量
	trafficData := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"uuid": uuid,
				"u":    1024000,
				"d":    2048000,
			},
		},
	}

	resp, err := http.Post(srv.URL()+"/api/v2/server/UniProxy/push", "application/json", toJsonReader(trafficData))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 验证流量已更新
	stats := srv.GetStats(uuid)
	assert.Equal(t, int64(1024000), stats.Upload)
	assert.Equal(t, int64(2048000), stats.Download)
}

func TestNodeRegister(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)
	ctx := context.Background()

	err = srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 注册节点
	registerData := map[string]interface{}{
		"auth_key": "test-key",
		"name":     "test-node",
		"host":     "192.168.1.1",
		"port":     443,
	}

	resp, err := http.Post(srv.URL()+"/api/v2/node/register", "application/json", toJsonReader(registerData))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Equal(t, float64(1), result["node_id"])
	assert.NotEmpty(t, result["api_key"])
}

func TestNodeHeartbeat(t *testing.T) {
	port, err := GetFreePort()
	require.NoError(t, err)

	srv := NewServer(port)
	ctx := context.Background()

	err = srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	resp, err := http.Post(srv.URL()+"/api/v2/node/heartbeat", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetFreePort(t *testing.T) {
	port1, err := GetFreePort()
	require.NoError(t, err)
	assert.Greater(t, port1, 0)

	port2, err := GetFreePort()
	require.NoError(t, err)
	assert.Greater(t, port2, 0)

	// 两次获取的端口可能相同（因为端口被释放了），但不应该有错误
}

// 辅助函数
func toJsonReader(v interface{}) *bytes.Reader {
	data, _ := json.Marshal(v)
	return bytes.NewReader(data)
}