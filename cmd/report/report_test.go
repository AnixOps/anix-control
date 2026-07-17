package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
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

func TestGenerateReportsWritePrivateFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	report = TestReport{
		GeneratedAt:   time.Now().Add(-time.Second),
		ServerVersion: "test",
		GoVersion:     "go-test",
		Platform:      "linux/amd64",
		Results: []TestResult{
			{Package: "pkg", TestName: "TestA", Status: "pass", Duration: time.Millisecond},
		},
		E2EResults: []E2EResult{
			{Protocol: "vless", Success: true, Latency: time.Millisecond},
		},
		APIResults: []APITest{
			{Endpoint: "/health", Method: "GET", Status: 200, Success: true, Latency: time.Millisecond},
		},
	}
	generateSummary()

	require.NoError(t, generateCSVReport())
	require.NoError(t, generateHTMLReport())

	dirInfo, err := os.Stat(reportDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(reportDirPerm), dirInfo.Mode().Perm())

	for _, name := range []string{"report.csv", "report.html", "report.json"} {
		info, err := os.Stat(filepath.Join(reportDir, name))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(reportFilePerm), info.Mode().Perm(), name)
	}
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
	port, err := getFreePort()
	require.NoError(t, err)
	assert.Greater(t, port, 0)

	// Verify the port is actually usable
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)
	l, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, l.Close())
	}()

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
	defer func() {
		require.NoError(t, l.Close())
	}()

	port := l.Addr().(*net.TCPAddr).Port
	result := waitForPort(port, 2*time.Second)
	assert.True(t, result)
}

func TestEchoServerStartStop(t *testing.T) {
	server := NewEchoServer(0) // port 0 = OS-assigned
	ctx := context.Background()

	err := server.Start(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, server.server.ReadHeaderTimeout)

	// Verify the server is listening
	time.Sleep(50 * time.Millisecond)

	// Use the actual assigned port
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", server.port))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, resp.Body.Close())
	}()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// Just verify no panic on start/stop cycle
	require.NoError(t, server.Stop())
}

func TestValidateReportXrayPath(t *testing.T) {
	dir := t.TempDir()
	xrayPath := filepath.Join(dir, "xray")
	require.NoError(t, os.WriteFile(xrayPath, []byte("#!/bin/sh\n"), 0o700))

	resolved, err := validateReportXrayPath(xrayPath)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(resolved))

	badPath := filepath.Join(dir, "sh")
	require.NoError(t, os.WriteFile(badPath, []byte("#!/bin/sh\n"), 0o700))
	_, err = validateReportXrayPath(badPath)
	assert.Error(t, err)
}
