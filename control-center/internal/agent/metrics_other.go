//go:build !linux

package agent

import (
	"context"
	"runtime"
	"time"
)

// collectHostInfo collects information about the host system (stub for non-Linux)
func (a *Agent) collectHostInfo() error {
	a.info.CPUCount = runtime.NumCPU()
	a.info.MemoryGB = 8.0 // Default
	a.info.DiskGB = 100.0 // Default
	return nil
}

// defaultMetricProvider provides system metrics (stub for non-Linux)
func (a *Agent) defaultMetricProvider() (*Metrics, error) {
	return &Metrics{
		Timestamp:    time.Now().Unix(),
		CPUUsage:     0.0,
		MemoryUsage:  0.0,
		MemoryTotal:  8 * 1024 * 1024 * 1024,
		MemoryUsed:   4 * 1024 * 1024 * 1024,
		DiskUsage:    0.0,
		DiskTotal:    100 * 1024 * 1024 * 1024,
		DiskUsed:     50 * 1024 * 1024 * 1024,
		LoadAvg1:     0.0,
		LoadAvg5:     0.0,
		LoadAvg15:    0.0,
		Uptime:       0,
		ProcessCount: 0,
	}, nil
}

// collectAndSendMetrics collects metrics and sends to control center (stub for non-Linux)
func (a *Agent) collectAndSendMetrics(ctx context.Context) error {
	if a.metricProvider == nil {
		return nil
	}
	metrics, err := a.metricProvider()
	if err != nil {
		return err
	}
	_ = metrics
	return nil
}

// pollAndExecuteCommands polls for pending commands (stub for non-Linux)
func (a *Agent) pollAndExecuteCommands(ctx context.Context) error {
	return nil
}
