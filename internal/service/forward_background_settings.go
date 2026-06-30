package service

import (
	"log"
	"strings"
	"time"

	appconfig "github.com/anixops/v2board/internal/config"
)

type forwardRuntimeJobExecutorSettings struct {
	PollInterval     time.Duration
	IdlePollInterval time.Duration
	ErrorLogInterval time.Duration
	BatchSize        int
	Timeout          time.Duration
}

type forwardGostStatsWorkerSettings struct {
	PollInterval     time.Duration
	IdlePollInterval time.Duration
	ErrorLogInterval time.Duration
}

type forwardLatencyProberSettings struct {
	PollInterval     time.Duration
	IdlePollInterval time.Duration
	ErrorLogInterval time.Duration
	Dials            int
	DialTimeout      time.Duration
	Concurrency      int
	Retention        time.Duration
}

func loadForwardRuntimeJobExecutorSettings() forwardRuntimeJobExecutorSettings {
	settings := forwardRuntimeJobExecutorSettings{
		PollInterval:     defaultForwardRuntimeJobPollInterval,
		IdlePollInterval: defaultForwardRuntimeJobIdlePollInterval,
		ErrorLogInterval: defaultForwardRuntimeJobErrorLogInterval,
		BatchSize:        defaultForwardRuntimeJobBatchSize,
		Timeout:          defaultForwardRuntimeJobTimeout,
	}

	cfg := appconfig.Get()
	if cfg == nil {
		return settings
	}

	settings.PollInterval = loadForwardBackgroundInterval(
		cfg.ForwardRuntime.Jobs.PollInterval,
		defaultForwardRuntimeJobPollInterval,
		"forward_runtime.jobs.poll_interval",
	)
	settings.IdlePollInterval = normalizeForwardIdlePollInterval(
		settings.PollInterval,
		loadForwardBackgroundInterval(
			cfg.ForwardRuntime.Jobs.IdlePollInterval,
			defaultForwardRuntimeJobIdlePollInterval,
			"forward_runtime.jobs.idle_poll_interval",
		),
	)
	settings.ErrorLogInterval = loadForwardBackgroundInterval(
		cfg.ForwardRuntime.Jobs.ErrorLogInterval,
		defaultForwardRuntimeJobErrorLogInterval,
		"forward_runtime.jobs.error_log_interval",
	)
	if cfg.ForwardRuntime.Jobs.BatchSize > 0 {
		settings.BatchSize = cfg.ForwardRuntime.Jobs.BatchSize
	} else if cfg.ForwardRuntime.Jobs.BatchSize < 0 {
		log.Printf(
			"invalid %s=%d, using default %d",
			"forward_runtime.jobs.batch_size",
			cfg.ForwardRuntime.Jobs.BatchSize,
			defaultForwardRuntimeJobBatchSize,
		)
	}
	if cfg.ForwardRuntime.Jobs.TimeoutSeconds > 0 {
		settings.Timeout = time.Duration(cfg.ForwardRuntime.Jobs.TimeoutSeconds) * time.Second
	} else if cfg.ForwardRuntime.Jobs.TimeoutSeconds < 0 {
		log.Printf(
			"invalid %s=%d, using default %s",
			"forward_runtime.jobs.timeout_seconds",
			cfg.ForwardRuntime.Jobs.TimeoutSeconds,
			defaultForwardRuntimeJobTimeout,
		)
	}

	return settings
}

func loadForwardGostStatsWorkerSettings() forwardGostStatsWorkerSettings {
	settings := forwardGostStatsWorkerSettings{
		PollInterval:     defaultForwardGostStatsPollInterval,
		IdlePollInterval: defaultForwardGostStatsIdlePollInterval,
		ErrorLogInterval: defaultForwardGostStatsErrorLogInterval,
	}

	cfg := appconfig.Get()
	if cfg == nil {
		return settings
	}

	settings.PollInterval = loadForwardBackgroundInterval(
		cfg.ForwardRuntime.GostStats.PollInterval,
		defaultForwardGostStatsPollInterval,
		"forward_runtime.gost_stats.poll_interval",
	)
	settings.IdlePollInterval = normalizeForwardIdlePollInterval(
		settings.PollInterval,
		loadForwardBackgroundInterval(
			cfg.ForwardRuntime.GostStats.IdlePollInterval,
			defaultForwardGostStatsIdlePollInterval,
			"forward_runtime.gost_stats.idle_poll_interval",
		),
	)
	settings.ErrorLogInterval = loadForwardBackgroundInterval(
		cfg.ForwardRuntime.GostStats.ErrorLogInterval,
		defaultForwardGostStatsErrorLogInterval,
		"forward_runtime.gost_stats.error_log_interval",
	)

	return settings
}

func loadForwardLatencyProberSettings() forwardLatencyProberSettings {
	settings := forwardLatencyProberSettings{
		PollInterval:     defaultForwardLatencyBucketInterval,
		IdlePollInterval: defaultForwardLatencyIdleInterval,
		ErrorLogInterval: defaultForwardLatencyErrorLogInterval,
		Dials:            defaultForwardLatencyDialsPerProbe,
		DialTimeout:      defaultForwardLatencyDialTimeout,
		Concurrency:      defaultForwardLatencyConcurrency,
		Retention:        time.Duration(defaultForwardLatencyRetentionDays) * 24 * time.Hour,
	}

	cfg := appconfig.Get()
	if cfg == nil {
		return settings
	}

	settings.PollInterval = loadForwardBackgroundInterval(
		cfg.ForwardRuntime.Latency.PollInterval,
		defaultForwardLatencyBucketInterval,
		"forward_runtime.latency.poll_interval",
	)
	settings.IdlePollInterval = normalizeForwardIdlePollInterval(
		settings.PollInterval,
		loadForwardBackgroundInterval(
			cfg.ForwardRuntime.Latency.IdlePollInterval,
			defaultForwardLatencyIdleInterval,
			"forward_runtime.latency.idle_poll_interval",
		),
	)
	settings.ErrorLogInterval = loadForwardBackgroundInterval(
		cfg.ForwardRuntime.Latency.ErrorLogInterval,
		defaultForwardLatencyErrorLogInterval,
		"forward_runtime.latency.error_log_interval",
	)
	settings.DialTimeout = loadForwardBackgroundInterval(
		cfg.ForwardRuntime.Latency.DialTimeout,
		defaultForwardLatencyDialTimeout,
		"forward_runtime.latency.dial_timeout",
	)
	if cfg.ForwardRuntime.Latency.Dials > 0 {
		settings.Dials = cfg.ForwardRuntime.Latency.Dials
	} else if cfg.ForwardRuntime.Latency.Dials < 0 {
		log.Printf("invalid %s=%d, using default %d", "forward_runtime.latency.dials", cfg.ForwardRuntime.Latency.Dials, defaultForwardLatencyDialsPerProbe)
	}
	if cfg.ForwardRuntime.Latency.Concurrency > 0 {
		settings.Concurrency = cfg.ForwardRuntime.Latency.Concurrency
	} else if cfg.ForwardRuntime.Latency.Concurrency < 0 {
		log.Printf("invalid %s=%d, using default %d", "forward_runtime.latency.concurrency", cfg.ForwardRuntime.Latency.Concurrency, defaultForwardLatencyConcurrency)
	}
	if cfg.ForwardRuntime.Latency.RetentionDays > 0 {
		settings.Retention = time.Duration(cfg.ForwardRuntime.Latency.RetentionDays) * 24 * time.Hour
	} else if cfg.ForwardRuntime.Latency.RetentionDays < 0 {
		log.Printf("invalid %s=%d, using default %d", "forward_runtime.latency.retention_days", cfg.ForwardRuntime.Latency.RetentionDays, defaultForwardLatencyRetentionDays)
	}

	return settings
}

func loadForwardBackgroundInterval(raw string, fallback time.Duration, key string) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}

	value, err := time.ParseDuration(trimmed)
	if err != nil || value <= 0 {
		log.Printf("invalid %s=%q, using default %s", key, trimmed, fallback)
		return fallback
	}
	return value
}
