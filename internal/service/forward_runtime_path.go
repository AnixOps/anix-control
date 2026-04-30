package service

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveForwardRuntimeWorkingDir(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return absForwardRuntimePath(trimmed)
}

func resolveForwardRuntimeFilePath(workingDir, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}

	baseCandidate := absForwardRuntimePath(trimmed)
	resolvedWorkingDir := resolveForwardRuntimeWorkingDir(workingDir)
	if resolvedWorkingDir == "" {
		return baseCandidate
	}

	joinedCandidate := absForwardRuntimePath(filepath.Join(resolvedWorkingDir, trimmed))
	if pathExists(baseCandidate) {
		return baseCandidate
	}
	if pathExists(joinedCandidate) {
		return joinedCandidate
	}

	if strings.Contains(trimmed, string(filepath.Separator)) || strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return baseCandidate
	}
	return joinedCandidate
}

func resolveForwardRuntimeEnvPath(workingDir, raw string) string {
	return resolveForwardRuntimeFilePath(workingDir, raw)
}

func absForwardRuntimePath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(trimmed)
}

var pathExists = func(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	_, err := os.Stat(trimmed)
	return err == nil
}
