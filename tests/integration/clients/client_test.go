package clients

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseClient(t *testing.T) {
	t.Run("NewBaseClient", func(t *testing.T) {
		client := NewBaseClient(ClientXray)
		assert.Equal(t, "xray", client.Name())
		assert.Equal(t, ClientXray, client.Type())
		assert.Equal(t, StatusStopped, client.Status())
	})

	t.Run("WithOptions", func(t *testing.T) {
		client := NewBaseClient(ClientMihomo,
			WithName("test-client"),
			WithConfig("/path/to/config.yaml"),
			WithPorts(8080, 1080, 7890, 9090),
		)

		assert.Equal(t, "test-client", client.Name())
		assert.Equal(t, ClientMihomo, client.Type())
		assert.Equal(t, "/path/to/config.yaml", client.configPath)
		assert.Equal(t, 8080, client.httpPort)
		assert.Equal(t, 1080, client.socksPort)
		assert.Equal(t, 7890, client.mixedPort)
		assert.Equal(t, 9090, client.apiPort)
	})

	t.Run("ProxyAddr", func(t *testing.T) {
		client := NewBaseClient(ClientXray, WithPorts(0, 10808, 0, 0))
		assert.Equal(t, "socks5://127.0.0.1:10808", client.ProxyAddr())

		client2 := NewBaseClient(ClientMihomo, WithPorts(0, 0, 7890, 0))
		assert.Equal(t, "127.0.0.1:7890", client2.ProxyAddr())
	})

	t.Run("HTTPProxyAddr", func(t *testing.T) {
		client := NewBaseClient(ClientXray, WithPorts(10809, 0, 0, 0))
		assert.Equal(t, "http://127.0.0.1:10809", client.HTTPProxyAddr())
	})

	t.Run("SocksProxyAddr", func(t *testing.T) {
		client := NewBaseClient(ClientXray, WithPorts(0, 10808, 0, 0))
		assert.Equal(t, "socks5://127.0.0.1:10808", client.SocksProxyAddr())
	})

	t.Run("SetConfig", func(t *testing.T) {
		client := NewBaseClient(ClientXray)
		client.SetConfig("/new/config.json")
		assert.Equal(t, "/new/config.json", client.configPath)
	})
}

func TestXrayClient(t *testing.T) {
	t.Run("NewXrayClient", func(t *testing.T) {
		client := NewXrayClient()
		assert.Equal(t, "xray", client.Name())
		assert.Equal(t, ClientXray, client.Type())
	})

	t.Run("StartWithoutConfig", func(t *testing.T) {
		client := NewXrayClient()
		ctx := context.Background()
		err := client.Start(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config path not set")
	})

	t.Run("StartWithNonExistentConfig", func(t *testing.T) {
		client := NewXrayClient(WithConfig("/nonexistent/config.json"))
		ctx := context.Background()
		err := client.Start(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config file not found")
	})
}

func TestMihomoClient(t *testing.T) {
	t.Run("NewMihomoClient", func(t *testing.T) {
		client := NewMihomoClient()
		assert.Equal(t, "mihomo", client.Name())
		assert.Equal(t, ClientMihomo, client.Type())
	})

	t.Run("StartWithoutConfig", func(t *testing.T) {
		client := NewMihomoClient()
		ctx := context.Background()
		err := client.Start(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config path not set")
	})
}

func TestManager(t *testing.T) {
	t.Run("NewManager", func(t *testing.T) {
		m := NewManager()
		assert.NotNil(t, m)
		assert.Empty(t, m.List())
	})

	t.Run("AddAndGet", func(t *testing.T) {
		m := NewManager()
		client := NewXrayClient(WithName("test-xray"))

		err := m.Add("test-xray", client)
		require.NoError(t, err)

		got, err := m.Get("test-xray")
		require.NoError(t, err)
		assert.Equal(t, client, got)
	})

	t.Run("AddDuplicate", func(t *testing.T) {
		m := NewManager()
		client := NewXrayClient()

		err := m.Add("xray", client)
		require.NoError(t, err)

		err = m.Add("xray", client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("GetNotFound", func(t *testing.T) {
		m := NewManager()
		_, err := m.Get("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Remove", func(t *testing.T) {
		m := NewManager()
		client := NewXrayClient()

		err := m.Add("xray", client)
		require.NoError(t, err)

		err = m.Remove("xray")
		require.NoError(t, err)

		_, err = m.Get("xray")
		assert.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		m := NewManager()
		m.Add("xray", NewXrayClient())
		m.Add("mihomo", NewMihomoClient())

		list := m.List()
		assert.Len(t, list, 2)
		assert.Contains(t, list, "xray")
		assert.Contains(t, list, "mihomo")
	})

	t.Run("Status", func(t *testing.T) {
		m := NewManager()
		m.Add("xray", NewXrayClient())
		m.Add("mihomo", NewMihomoClient())

		status := m.Status()
		assert.Len(t, status, 2)
		assert.Equal(t, StatusStopped, status["xray"])
		assert.Equal(t, StatusStopped, status["mihomo"])
	})
}

func TestClientStatus(t *testing.T) {
	tests := []struct {
		status   ClientStatus
		expected string
	}{
		{StatusStopped, "stopped"},
		{StatusStarting, "starting"},
		{StatusRunning, "running"},
		{StatusError, "error"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

func TestClientType(t *testing.T) {
	tests := []struct {
		clientType ClientType
		expected   string
	}{
		{ClientXray, "xray"},
		{ClientMihomo, "mihomo"},
	}

	for _, tt := range tests {
		t.Run(string(tt.clientType), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.clientType))
		})
	}
}

func TestManagerWatch(t *testing.T) {
	m := NewManager()
	m.Add("xray", NewXrayClient())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动监控（非阻塞）
	go m.Watch(ctx, 100*time.Millisecond, func(name string, healthy bool) {
		// 回调函数
	})

	// 等待一个周期
	time.Sleep(150 * time.Millisecond)

	// 取消后应该停止
	cancel()
	time.Sleep(50 * time.Millisecond)
}