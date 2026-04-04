package echo

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEchoServer(t *testing.T) {
	srv := NewEchoServer(0) // 自动分配端口
	assert.NotNil(t, srv)
	assert.Equal(t, 0, srv.Port())
}

func TestEchoServerLifecycle(t *testing.T) {
	srv := NewEchoServer(0)
	ctx := context.Background()

	// 启动
	err := srv.Start(ctx)
	require.NoError(t, err)
	assert.True(t, srv.IsRunning())
	assert.Greater(t, srv.Port(), 0)

	// 检查 URL
	assert.Contains(t, srv.URL(), "127.0.0.1")

	// 停止
	err = srv.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, srv.IsRunning())
}

func TestEchoServerEndpoints(t *testing.T) {
	srv := NewEchoServer(0)
	ctx := context.Background()

	err := srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	t.Run("/echo", func(t *testing.T) {
		resp, err := http.Get(srv.URL() + "/echo?test=1")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "v1", resp.Header.Get("X-Echo-Server"))

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Echo Response")
		assert.Contains(t, string(body), "GET")
	})

	t.Run("/ping", func(t *testing.T) {
		resp, err := http.Get(srv.URL() + "/ping")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "pong", string(body))
	})

	t.Run("/status", func(t *testing.T) {
		resp, err := http.Get(srv.URL() + "/status")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	})

	t.Run("/generate_204", func(t *testing.T) {
		resp, err := http.Get(srv.URL() + "/generate_204")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}

func TestEchoServerStats(t *testing.T) {
	srv := NewEchoServer(0)
	ctx := context.Background()

	err := srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 等待服务器启动
	time.Sleep(50 * time.Millisecond)

	// 初始统计
	req, in, out := srv.Stats()
	assert.Equal(t, int64(0), req)

	// 发送请求
	resp1, err := http.Get(srv.URL() + "/ping")
	require.NoError(t, err)
	resp1.Body.Close()

	resp2, err := http.Get(srv.URL() + "/echo")
	require.NoError(t, err)
	resp2.Body.Close()

	// 检查统计
	req, in, out = srv.Stats()
	assert.Equal(t, int64(2), req)
	// bytesIn/bytesOut 可能是 0，因为 ping 和简单的 echo 请求没有 body
	// 所以我们只检查请求计数
	_ = in
	_ = out
}

func TestEchoServerPostBody(t *testing.T) {
	srv := NewEchoServer(0)
	ctx := context.Background()

	err := srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 发送 POST 请求
	resp, err := http.Post(srv.URL()+"/echo", "text/plain",
		nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "POST")
}

func TestHTTPEchoServer(t *testing.T) {
	srv := NewHTTPEchoServer(0)
	ctx := context.Background()

	err := srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop()

	assert.Greater(t, srv.Port(), 0)
	assert.Contains(t, srv.URL(), "127.0.0.1")

	// 发送请求
	resp, err := http.Get(srv.URL() + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Echo")
	assert.Contains(t, string(body), "GET")
}

func TestTCPEchoServer(t *testing.T) {
	srv := NewTCPEchoServer(0)
	ctx := context.Background()

	err := srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop()

	assert.Greater(t, srv.Port(), 0)
	assert.Contains(t, srv.Addr(), "127.0.0.1")
}

func TestEchoServerRestart(t *testing.T) {
	srv := NewEchoServer(0)
	ctx := context.Background()

	// 第一次启动
	err := srv.Start(ctx)
	require.NoError(t, err)

	port1 := srv.Port()
	assert.Greater(t, port1, 0)

	// 停止
	err = srv.Stop(ctx)
	require.NoError(t, err)

	// 等待端口释放 (TCP TIME_WAIT)
	time.Sleep(100 * time.Millisecond)

	// 重新启动（应该使用新端口）
	err = srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 新端口
	port2 := srv.Port()
	assert.Greater(t, port2, 0)
}

func TestEchoServerConcurrent(t *testing.T) {
	srv := NewEchoServer(0)
	ctx := context.Background()

	err := srv.Start(ctx)
	require.NoError(t, err)
	defer srv.Stop(ctx)

	// 并发发送请求
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			resp, err := http.Get(srv.URL() + "/ping")
			if err == nil {
				resp.Body.Close()
			}
			done <- true
		}()
	}

	// 等待所有请求完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 检查统计
	req, _, _ := srv.Stats()
	assert.Equal(t, int64(10), req)
}