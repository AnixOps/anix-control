package config

import (
	"log"
	"strings"
	"sync"
	"testing"
)

// resetConfig clears the process configuration and Load's once (tests only).
func resetConfig() {
	cfg = nil
	once = sync.Once{}
}

// captureLog collects the standard logger's lines until restore.
func captureLog(t *testing.T, lines *[]string) (restore func()) {
	t.Helper()
	writer := &lineCollector{lines: lines}
	previous, flags := log.Writer(), log.Flags()
	log.SetOutput(writer)
	log.SetFlags(0)
	return func() {
		log.SetOutput(previous)
		log.SetFlags(flags)
	}
}

type lineCollector struct{ lines *[]string }

func (c *lineCollector) Write(data []byte) (int, error) {
	*c.lines = append(*c.lines, strings.TrimRight(string(data), "\n"))
	return len(data), nil
}
