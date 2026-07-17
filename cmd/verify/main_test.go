package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveVerifyToken(t *testing.T) {
	token, err := resolveVerifyToken("  token-123  ")
	if err != nil {
		t.Fatalf("resolveVerifyToken() error = %v", err)
	}
	if token != "token-123" {
		t.Fatalf("resolveVerifyToken() = %q, want token-123", token)
	}

	for _, raw := range []string{"", "abc/def", "abc?def", "abc#def"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := resolveVerifyToken(raw); err == nil {
				t.Fatalf("resolveVerifyToken(%q) expected error", raw)
			}
		})
	}
}

func TestResolvePanelBaseURLRestrictsToLoopback(t *testing.T) {
	base, err := resolvePanelBaseURL("")
	if err != nil {
		t.Fatalf("resolvePanelBaseURL(default) error = %v", err)
	}
	if got := buildVerifyURL(base, "s", "token-123"); got != "http://127.0.0.1:8080/s/token-123" {
		t.Fatalf("buildVerifyURL() = %q", got)
	}

	withPath, err := resolvePanelBaseURL("http://localhost:18080/panel?ignored=1#frag")
	if err != nil {
		t.Fatalf("resolvePanelBaseURL(path) error = %v", err)
	}
	if got := buildVerifyURL(withPath, "health"); got != "http://localhost:18080/panel/health" {
		t.Fatalf("buildVerifyURL(withPath) = %q", got)
	}

	for _, raw := range []string{"ftp://127.0.0.1:8080", "http://example.com:8080", "http://192.0.2.10:8080"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := resolvePanelBaseURL(raw); err == nil {
				t.Fatalf("resolvePanelBaseURL(%q) expected error", raw)
			}
		})
	}
}

func TestResolveXrayPathAllowsOnlyLocalXrayBinaryNames(t *testing.T) {
	dir := t.TempDir()
	xrayPath := filepath.Join(dir, "xray")
	if err := os.WriteFile(xrayPath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveXrayPath(xrayPath)
	if err != nil {
		t.Fatalf("resolveXrayPath() error = %v", err)
	}
	if !filepath.IsAbs(resolved) {
		t.Fatalf("resolveXrayPath() = %q, want absolute path", resolved)
	}

	disallowed := filepath.Join(dir, "sh")
	if err := os.WriteFile(disallowed, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveXrayPath(disallowed); err == nil {
		t.Fatal("resolveXrayPath(disallowed) expected error")
	}
}

func TestWriteVerifyConfigUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := writeVerifyConfig(path, "{}"); err != nil {
		t.Fatalf("writeVerifyConfig() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("permissions = %v, want %v", got, want)
	}
}
