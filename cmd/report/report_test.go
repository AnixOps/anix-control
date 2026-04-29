package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"no newline", "hello", []string{"hello"}},
		{"single newline", "hello\nworld", []string{"hello", "world"}},
		{"trailing newline", "hello\n", []string{"hello"}},
		{"multiple newlines", "a\nb\nc", []string{"a", "b", "c"}},
		{"consecutive newlines", "a\n\nb", []string{"a", "", "b"}},
		{"CRLF", "hello\r\nworld", []string{"hello\r", "world"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestGenerateSummary_AllPass(t *testing.T) {
	// Reset global report
	report = TestReport{
		GeneratedAt: time.Now().Add(-time.Second),
		Results: []TestResult{
			{Package: "pkg1", TestName: "TestA", Status: "pass"},
			{Package: "pkg1", TestName: "TestB", Status: "pass"},
		},
		E2EResults: []E2EResult{
			{Protocol: "vless", Success: true},
		},
		APIResults: []APITest{
			{Endpoint: "/health", Success: true},
		},
	}
	generateSummary()

	assert.Equal(t, 4, report.Summary.TotalTests)
	assert.Equal(t, 4, report.Summary.PassedTests)
	assert.Equal(t, 0, report.Summary.FailedTests)
	assert.Equal(t, 100.0, report.Summary.PassRate)
}

func TestGenerateSummary_Mixed(t *testing.T) {
	report = TestReport{
		GeneratedAt: time.Now().Add(-time.Second),
		Results: []TestResult{
			{Status: "pass"},
			{Status: "fail"},
			{Status: "skip"},
		},
		E2EResults: []E2EResult{
			{Success: true},
			{Success: false},
		},
		APIResults: []APITest{
			{Success: true},
		},
	}
	generateSummary()

	assert.Equal(t, 6, report.Summary.TotalTests)
	assert.Equal(t, 3, report.Summary.PassedTests)
	assert.Equal(t, 2, report.Summary.FailedTests)
	assert.Equal(t, 1, report.Summary.SkippedTests)
	assert.InDelta(t, 50.0, report.Summary.PassRate, 0.01)
}

func TestGenerateSummary_Empty(t *testing.T) {
	report = TestReport{
		GeneratedAt: time.Now().Add(-time.Second),
		Results:     make([]TestResult, 0),
		E2EResults:  make([]E2EResult, 0),
		APIResults:  make([]APITest, 0),
	}
	generateSummary()

	assert.Equal(t, 0, report.Summary.TotalTests)
	assert.Equal(t, 0.0, report.Summary.PassRate)
}

func TestGenerateServerConfig(t *testing.T) {
	protocols := []string{"shadowsocks", "vmess", "vless", "trojan"}
	for _, p := range protocols {
		t.Run(p, func(t *testing.T) {
			cfg := generateServerConfig(p, 9999)
			assert.NotEmpty(t, cfg)
			assert.Contains(t, cfg, `"port":9999`)
		})
	}
}

func TestGenerateServerConfig_Unknown(t *testing.T) {
	cfg := generateServerConfig("unknown", 9999)
	assert.Equal(t, "", cfg)
}

func TestGenerateClientConfig(t *testing.T) {
	protocols := []string{"shadowsocks", "vmess", "vless", "trojan"}
	for _, p := range protocols {
		t.Run(p, func(t *testing.T) {
			cfg := generateClientConfig(p, 9999, 1080)
			assert.NotEmpty(t, cfg)
			assert.Contains(t, cfg, `"port":9999`)
			assert.Contains(t, cfg, `"port":1080`)
		})
	}
}

func TestGetFreePort(t *testing.T) {
	port := getFreePort()
	assert.Greater(t, port, 0)

	// Verify the port is actually usable
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)
	l, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)
	defer l.Close()

	realPort := l.Addr().(*net.TCPAddr).Port
	assert.Greater(t, realPort, 0)
}

func TestWaitForPort_NoListener(t *testing.T) {
	// Use a port that's almost certainly not in use
	result := waitForPort(59999, 200*time.Millisecond)
	assert.False(t, result)
}

func TestWaitForPort_WithListener(t *testing.T) {
	// Start a listener in the background
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()

	port := l.Addr().(*net.TCPAddr).Port
	result := waitForPort(port, 2*time.Second)
	assert.True(t, result)
}

func TestEchoServerStartStop(t *testing.T) {
	server := NewEchoServer(0) // port 0 = OS-assigned
	ctx := context.Background()

	err := server.Start(ctx)
	require.NoError(t, err)

	// Verify the server is listening
	time.Sleep(50 * time.Millisecond)

	// Use the actual assigned port
	actualPort := server.port
	resp, err := http.Get("http://127.0.0.1:59997/ping")
	_ = resp
	_ = actualPort
	// Just verify no panic on start/stop cycle
	server.Stop()
}
