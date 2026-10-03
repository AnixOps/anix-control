package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type hostObservation struct {
	Mode           string     `json:"mode"`
	Effective      string     `json:"effective"`
	NativeTotal    uint64     `json:"native_total"`
	NativeErrors   uint64     `json:"native_errors"`
	ShadowTotal    uint64     `json:"shadow_total"`
	ShadowMismatch uint64     `json:"shadow_mismatch"`
	ShadowErrors   uint64     `json:"shadow_errors"`
	ShadowSkipped  uint64     `json:"shadow_skipped"`
	MismatchRate   float64    `json:"mismatch_rate"`
	LastMismatchAt *time.Time `json:"last_mismatch_at"`
}

type routeModeEntry struct {
	RouteID         string           `json:"route_id"`
	Method          string           `json:"method"`
	Path            string           `json:"path"`
	Catalog         string           `json:"catalog"`
	Configured      string           `json:"configured"`
	Effective       string           `json:"effective"`
	AllowedModes    []string         `json:"allowed_modes"`
	Locked          string           `json:"locked"`
	Host            *hostObservation `json:"host"`
	MismatchSamples *struct {
		Stored         int64     `json:"stored"`
		LastObservedAt time.Time `json:"last_observed_at"`
	} `json:"mismatch_samples"`
}

type packageRouteModes struct {
	PackageID      string           `json:"package_id"`
	Version        string           `json:"version"`
	Enabled        bool             `json:"enabled"`
	ConfigRevision int64            `json:"config_revision"`
	Error          string           `json:"error"`
	Routes         []routeModeEntry `json:"routes"`
}

type routeModeRevision struct {
	PackageID string    `json:"package_id"`
	RouteID   string    `json:"route_id"`
	Action    string    `json:"action"`
	FromMode  string    `json:"from_mode"`
	ToMode    string    `json:"to_mode"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type mismatchSample struct {
	SampleID     string `json:"sample_id"`
	PackageID    string `json:"package_id"`
	RouteID      string `json:"route_id"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	LegacyStatus int    `json:"legacy_status"`
	NativeStatus int    `json:"native_status"`
	Diff         []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"diff"`
	RequestID  string    `json:"request_id"`
	ObservedAt time.Time `json:"observed_at"`
}

func (c *Client) routeModes(ctx context.Context, token, packageID string) (map[string]routeModeEntry, error) {
	var data struct {
		Packages []packageRouteModes `json:"packages"`
	}
	if err := c.kernel(ctx, http.MethodGet, "/api/v4/kernel/route-modes", q("package_id", packageID), nil, &data, token); err != nil {
		return nil, err
	}
	entries := map[string]routeModeEntry{}
	for _, pkg := range data.Packages {
		if pkg.Error != "" {
			return nil, fmt.Errorf("route modes of %s: %s", pkg.PackageID, pkg.Error)
		}
		for _, route := range pkg.Routes {
			entries[route.RouteID] = route
		}
	}
	return entries, nil
}

func (c *Client) revisions(ctx context.Context, token, packageID string) ([]routeModeRevision, error) {
	var data struct {
		Revisions []routeModeRevision `json:"revisions"`
	}
	err := c.kernel(ctx, http.MethodGet, "/api/v4/kernel/route-modes/revisions", q("package_id", packageID, "limit", "1000"), nil, &data, token)
	return data.Revisions, err
}

func (c *Client) mismatches(ctx context.Context, token, packageID, routeID string) ([]mismatchSample, error) {
	var data struct {
		Samples []mismatchSample `json:"samples"`
	}
	err := c.kernel(ctx, http.MethodGet, "/api/v4/kernel/route-modes/mismatches", q("package_id", packageID, "route_id", routeID, "limit", "100"), nil, &data, token)
	return data.Samples, err
}

// shadowSince returns, per route, when its current shadow period started:
// the newest revision that switched it to shadow, if no later revision
// switched it away. Revisions come newest first.
func shadowSince(revisions []routeModeRevision) map[string]time.Time {
	since := map[string]time.Time{}
	decided := map[string]bool{}
	for _, revision := range revisions {
		if decided[revision.RouteID] {
			continue
		}
		decided[revision.RouteID] = true
		if revision.ToMode == "shadow" {
			since[revision.RouteID] = revision.CreatedAt
		}
	}
	return since
}

func (c *Client) setModes(ctx context.Context, token, packageID string, routes []string, mode, reason string, confirm bool) error {
	body := map[string]any{"package_id": packageID, "routes": routes, "mode": mode, "reason": reason, "confirm": confirm}
	var result struct {
		Changes []struct {
			RouteID, From, To string
		} `json:"changes"`
		Skipped []struct {
			RouteID string `json:"route_id"`
			Reason  string `json:"reason"`
		} `json:"skipped"`
		ConfigRevision int64 `json:"config_revision"`
	}
	if err := c.kernel(ctx, http.MethodPost, "/api/v4/kernel/route-modes", nil, body, &result, token); err != nil {
		return err
	}
	fmt.Printf("%s: %d route(s) -> %s (configuration revision %d)\n", packageID, len(result.Changes), mode, result.ConfigRevision)
	for _, skip := range result.Skipped {
		fmt.Printf("  skipped %s: %s\n", skip.RouteID, skip.Reason)
	}
	return nil
}

// switchShadow puts the batch's native-flagged read routes in shadow on
// the rehearsal instance. Routes already in shadow keep their start time.
func switchShadow(ctx context.Context, common commonFlags) error {
	batch, err := batchByNumber(common.batch)
	if err != nil {
		return err
	}
	routes, err := batchRoutes(batch)
	if err != nil {
		return err
	}
	client := newClient(common.url)
	token, err := client.Login(ctx, common.adminEmail, common.adminPassword)
	if err != nil {
		return err
	}
	byPackage := map[string][]string{}
	for _, route := range routes {
		if route.Read() {
			byPackage[route.PackageID] = append(byPackage[route.PackageID], route.RouteID)
		}
	}
	for _, packageID := range batch.Packages {
		reads := byPackage[packageID]
		if len(reads) == 0 {
			fmt.Printf("%s: no native-flagged read route\n", packageID)
			continue
		}
		current, err := client.routeModes(ctx, token, packageID)
		if err != nil {
			return err
		}
		var pending []string
		for _, id := range reads {
			if current[id].Configured != "shadow" {
				pending = append(pending, id)
			}
		}
		if len(pending) == 0 {
			fmt.Printf("%s: %d read route(s) already in shadow\n", packageID, len(reads))
			continue
		}
		if err := client.setModes(ctx, token, packageID, pending, "shadow",
			fmt.Sprintf("staging rehearsal batch %d: shadow the read routes", batch.Number), false); err != nil {
			return err
		}
	}
	return nil
}

// rollbackBatch returns every package of the batch to legacy.
func rollbackBatch(ctx context.Context, common commonFlags) error {
	batch, err := batchByNumber(common.batch)
	if err != nil {
		return err
	}
	client := newClient(common.url)
	token, err := client.Login(ctx, common.adminEmail, common.adminPassword)
	if err != nil {
		return err
	}
	for _, packageID := range batch.Packages {
		body := map[string]string{"package_id": packageID, "reason": fmt.Sprintf("staging rehearsal batch %d: rollback", batch.Number)}
		if err := client.kernel(ctx, http.MethodPost, "/api/v4/kernel/route-modes/rollback", nil, body, nil, token); err != nil {
			return err
		}
		fmt.Printf("%s: rolled back to legacy\n", packageID)
	}
	return nil
}

// nativeCommands is what an operator runs after the owner signs the batch
// off (H7); the rehearsal never runs it on the rehearsal instance.
func nativeCommands(batch Batch) []string {
	var commands []string
	for _, packageID := range batch.Packages {
		commands = append(commands, fmt.Sprintf(
			"scripts/staging/rehearse.sh routes set --package %s --mode native --reason %s --yes",
			packageID, shellQuote(fmt.Sprintf("batch %d signed off (H7): staging rehearsal passed", batch.Number))))
	}
	return commands
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'" }
