package agent

import (
	"bufio"
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// collectHostInfo collects information about the host system
func (a *Agent) collectHostInfo() error {
	// CPU count is already set
	a.info.CPUCount = runtime.NumCPU()

	// Get memory info
	if memTotal, memGB, err := getMemoryInfo(); err == nil {
		a.info.MemoryGB = memGB
		_ = memTotal
	}

	// Get disk info
	if diskGB, err := getDiskInfo(); err == nil {
		a.info.DiskGB = diskGB
	}

	return nil
}

// defaultMetricProvider provides system metrics
func (a *Agent) defaultMetricProvider() (*Metrics, error) {
	m := &Metrics{
		Timestamp: time.Now().Unix(),
	}

	// CPU usage
	if usage, err := getCPUUsage(); err == nil {
		m.CPUUsage = usage
	}

	// Memory metrics
	if total, used, usage, err := getMemoryMetrics(); err == nil {
		m.MemoryTotal = total
		m.MemoryUsed = used
		m.MemoryUsage = usage
	}

	// Disk metrics
	if total, used, usage, err := getDiskMetrics(); err == nil {
		m.DiskTotal = total
		m.DiskUsed = used
		m.DiskUsage = usage
	}

	// Load average
	if load1, load5, load15, err := getLoadAverage(); err == nil {
		m.LoadAvg1 = load1
		m.LoadAvg5 = load5
		m.LoadAvg15 = load15
	}

	// Uptime
	if uptime, err := getUptime(); err == nil {
		m.Uptime = uptime
	}

	// Process count
	if count, err := getProcessCount(); err == nil {
		m.ProcessCount = count
	}

	return m, nil
}

// collectAndSendMetrics collects metrics and sends to control center
func (a *Agent) collectAndSendMetrics(ctx context.Context) error {
	if a.metricProvider == nil {
		return nil
	}

	metrics, err := a.metricProvider()
	if err != nil {
		return err
	}

	// TODO: Send metrics to control center API
	_ = metrics
	return nil
}

// pollAndExecuteCommands polls for pending commands
func (a *Agent) pollAndExecuteCommands(ctx context.Context) error {
	// TODO: Poll commands from control center API
	return nil
}

// Platform-specific implementations

func getMemoryInfo() (uint64, float64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	var memTotal uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memKB, _ := strconv.ParseUint(fields[1], 10, 64)
				memTotal = memKB * 1024
				memGB := float64(memKB) / 1024 / 1024
				return memTotal, memGB, nil
			}
		}
	}

	return 0, 0, nil
}

func getDiskInfo() (float64, error) {
	// Simple implementation - assume root filesystem
	// In production, use syscall.Statfs
	return 100.0, nil // Default 100GB
}

func getCPUUsage() (float64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) >= 8 {
				idle, _ := strconv.ParseFloat(fields[4], 64)
				total := 0.0
				for i := 1; i < 8; i++ {
					v, _ := strconv.ParseFloat(fields[i], 64)
					total += v
				}
				if total > 0 {
					return (total - idle) / total * 100, nil
				}
			}
		}
	}

	return 0, nil
}

func getMemoryMetrics() (total, used uint64, usage float64, err error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer file.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		if len(fields) < 2 {
			continue
		}

		value, _ := strconv.ParseUint(fields[1], 10, 64)

		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = value * 1024
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailable = value * 1024
		}
	}

	total = memTotal
	used = memTotal - memAvailable
	if memTotal > 0 {
		usage = float64(used) / float64(memTotal) * 100
	}

	return total, used, usage, nil
}

func getDiskMetrics() (total, used uint64, usage float64, err error) {
	// Simple implementation
	// In production, use syscall.Statfs
	return 100 * 1024 * 1024 * 1024, 50 * 1024 * 1024 * 1024, 50.0, nil
}

func getLoadAverage() (load1, load5, load15 float64, err error) {
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 {
			load1, _ = strconv.ParseFloat(fields[0], 64)
			load5, _ = strconv.ParseFloat(fields[1], 64)
			load15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	return load1, load5, load15, nil
}

func getUptime() (uint64, error) {
	file, err := os.Open("/proc/uptime")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 1 {
			uptime, _ := strconv.ParseFloat(fields[0], 64)
			return uint64(uptime), nil
		}
	}

	return 0, nil
}

func getProcessCount() (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			name := entry.Name()
			if _, err := strconv.Atoi(name); err == nil {
				count++
			}
		}
	}

	return count, nil
}
