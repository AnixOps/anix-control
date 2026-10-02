package main

import (
	"context"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/shadowsamples"
)

// startShadowSampleCollector stores the sanitized shadow mismatch samples
// package hosts report in their Health details, at the health poll
// interval. Only details from a new poll are read.
func (rt *serverRuntime) startShadowSampleCollector(hosts pluginhost.HostStatsProvider) {
	collector := &shadowsamples.Collector{Route: service.ShadowSampleRoute}
	rt.workers.Go("shadow mismatch sample collector", func(ctx context.Context) {
		ticker := time.NewTicker(pluginHostHealthPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			db := database.Get()
			if db == nil {
				continue
			}
			collector.DB = db
			if _, err := collector.Collect(ctx, shadowSampleReports(hosts.Stats())); err != nil && ctx.Err() == nil {
				log.Printf("Shadow mismatch sample collection failed: %v", err)
			}
		}
	})
}

func shadowSampleReports(stats []pluginhost.HostStats) []shadowsamples.Report {
	reports := make([]shadowsamples.Report, 0, len(stats))
	for _, entry := range stats {
		if entry.HealthDetailsJSON == "" {
			continue
		}
		reports = append(reports, shadowsamples.Report{
			PackageID: entry.PackageID, Version: entry.Version, DetailsJSON: entry.HealthDetailsJSON, CheckedAt: entry.HealthCheckedAt,
		})
	}
	return reports
}
