package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/utils"
)

type replayOptions struct {
	MinRequests int
	MinDuration time.Duration
	// Rate is the request rate in requests per second; 0 spreads the
	// required requests over the required shadow duration.
	Rate float64
	Seed int64
}

// ReadResult is the outcome of one shadowed read route.
type ReadResult struct {
	RouteID   string         `json:"route_id"`
	PackageID string         `json:"package_id"`
	Path      string         `json:"path"`
	Variants  int            `json:"variants"`
	Sent      int            `json:"sent"`
	Statuses  map[string]int `json:"statuses"`
	// TransportErrors are requests that got no HTTP answer.
	TransportErrors int `json:"transport_errors"`
	// Host counters of the package host since it started (route-modes API).
	Compared   uint64 `json:"compared"`
	Mismatches uint64 `json:"mismatches"`
	Errors     uint64 `json:"native_errors"`
	Skipped    uint64 `json:"skipped"`
	// StoredSamples are mismatch samples observed since the shadow start.
	StoredSamples int            `json:"stored_samples"`
	Samples       []SampleDigest `json:"samples,omitempty"`
	Mode          string         `json:"mode"`
	ShadowSince   *time.Time     `json:"shadow_since,omitempty"`
	ShadowFor     string         `json:"shadow_duration"`
	ShadowSeconds float64        `json:"shadow_seconds"`
	SamplesAPI    string         `json:"samples_api"`
	Pass          bool           `json:"pass"`
	Reasons       []string       `json:"reasons,omitempty"`
	Skip          string         `json:"skip,omitempty"`
}

// SampleDigest summarizes one stored mismatch sample.
type SampleDigest struct {
	SampleID     string    `json:"sample_id"`
	Path         string    `json:"path"`
	LegacyStatus int       `json:"legacy_status"`
	NativeStatus int       `json:"native_status"`
	DiffPaths    []string  `json:"diff_paths"`
	RequestID    string    `json:"request_id"`
	ObservedAt   time.Time `json:"observed_at"`
}

// ReadsReport is reads.json.
type ReadsReport struct {
	Batch       int          `json:"batch"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  time.Time    `json:"finished_at"`
	MinRequests int          `json:"min_requests"`
	MinDuration string       `json:"min_shadow_duration"`
	Rate        float64      `json:"rate"`
	Routes      []ReadResult `json:"routes"`
	Personas    []string     `json:"persona_notes,omitempty"`
}

type readTarget struct {
	route    CatalogRoute
	variants []Req
	next     int
	result   *ReadResult
}

func replayReads(ctx context.Context, common commonFlags, options replayOptions) error {
	batch, err := batchByNumber(common.batch)
	if err != nil {
		return err
	}
	routes, err := batchRoutes(batch)
	if err != nil {
		return err
	}
	world, err := loadWorld(manifestPath)
	if err != nil {
		return err
	}
	client := newClient(common.url)
	adminToken, err := client.Login(ctx, common.adminEmail, common.adminPassword)
	if err != nil {
		return err
	}
	tokens, unavailable := personaTokens(ctx, client, world, common)
	report := ReadsReport{Batch: batch.Number, StartedAt: time.Now().UTC(), MinRequests: options.MinRequests,
		MinDuration: options.MinDuration.String(), Personas: unavailable}

	var targets []*readTarget
	// Fixed capacity: targets keep pointers into report.Routes.
	report.Routes = make([]ReadResult, 0, len(routes))
	for _, route := range routes {
		if !route.Read() {
			continue
		}
		result := &ReadResult{RouteID: route.RouteID, PackageID: route.PackageID, Path: route.Path, Statuses: map[string]int{},
			SamplesAPI: fmt.Sprintf("/api/v4/kernel/route-modes/mismatches?package_id=%s&route_id=%s", route.PackageID, route.RouteID)}
		report.Routes = append(report.Routes, *result)
		result = &report.Routes[len(report.Routes)-1]
		spec, ok := routeSpecs[route.RouteID]
		switch {
		case !ok:
			result.Skip = "no request generator"
		case spec.Skip != "":
			result.Skip = spec.Skip
		case spec.Reads == nil:
			result.Skip = "no read generator"
		}
		if result.Skip != "" {
			continue
		}
		var variants []Req
		for _, variant := range spec.Reads(world) {
			if _, ok := tokens[variant.Persona]; ok || variant.Persona == Anon {
				variants = append(variants, variant)
			}
		}
		result.Variants = len(variants)
		if len(variants) == 0 {
			result.Skip = "no usable request variant"
			continue
		}
		targets = append(targets, &readTarget{route: route, variants: variants, result: result})
	}
	if len(targets) == 0 {
		return writeJSON(filepath.Join(batchDir(common), "reads.json"), report)
	}

	rate := options.Rate
	if rate <= 0 {
		// Spread the required requests over the required time, with 25%
		// head room, so traffic covers the whole shadow period.
		rate = float64(len(targets)*options.MinRequests) * 1.25 / math.Max(options.MinDuration.Seconds(), 1)
		rate = math.Min(math.Max(rate, 0.5), 20)
	}
	report.Rate = math.Round(rate*100) / 100
	fmt.Printf("batch %d: replaying %d read routes at %.2f req/s until each has %d compared requests and %s in shadow\n",
		batch.Number, len(targets), rate, options.MinRequests, options.MinDuration)

	interval := time.Duration(float64(time.Second) / rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastObserve := time.Time{}
	observations := map[string]map[string]routeModeEntry{}
	since := map[string]time.Time{}
	satisfied := func(target *readTarget) bool {
		entry := observations[target.route.PackageID][target.route.RouteID]
		start, ok := since[target.route.RouteID]
		return entry.Host != nil && entry.Host.ShadowTotal >= uint64(options.MinRequests) && ok && time.Since(start) >= options.MinDuration // #nosec G115 -- --min-requests is a small positive flag value
	}
	cursor := 0
	for {
		if time.Since(lastObserve) > 20*time.Second {
			if err := observe(ctx, client, adminToken, batch, observations, since); err != nil {
				fmt.Fprintln(os.Stderr, "observe:", err)
			}
			lastObserve = time.Now()
			done, stuck := 0, 0
			for _, target := range targets {
				if satisfied(target) {
					done++
				} else if target.result.Sent >= 20*options.MinRequests+100 {
					stuck++
				}
			}
			fmt.Printf("%s progress: %d/%d routes satisfied, %d without enough compared requests after %d sends\n",
				time.Now().UTC().Format(time.TimeOnly), done, len(targets), stuck, 20*options.MinRequests+100)
			if done+stuck == len(targets) {
				break
			}
		}
		target := targets[cursor%len(targets)]
		cursor++
		if target.result.Sent >= 20*options.MinRequests+100 {
			if cursor%len(targets) == 0 {
				time.Sleep(200 * time.Millisecond) // every route capped: wait for the next observation
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		variant := target.variants[target.next%len(target.variants)]
		target.next++
		response, err := sendAs(ctx, client, world, common, tokens, variant, http.MethodGet)
		target.result.Sent++
		if err != nil {
			target.result.TransportErrors++
			continue
		}
		target.result.Statuses[fmt.Sprint(response.Status)]++
	}
	// Shadow runs finish in the background and reach the kernel with the
	// next health poll.
	time.Sleep(12 * time.Second)
	if err := observe(ctx, client, adminToken, batch, observations, since); err != nil {
		return err
	}
	for i := range report.Routes {
		result := &report.Routes[i]
		finishReadResult(ctx, client, adminToken, result, observations, since, options)
	}
	report.FinishedAt = time.Now().UTC()
	return writeJSON(filepath.Join(batchDir(common), "reads.json"), report)
}

func observe(ctx context.Context, client *Client, token string, batch Batch, observations map[string]map[string]routeModeEntry, since map[string]time.Time) error {
	for _, packageID := range batch.Packages {
		entries, err := client.routeModes(ctx, token, packageID)
		if err != nil {
			return err
		}
		observations[packageID] = entries
		revisions, err := client.revisions(ctx, token, packageID)
		if err != nil {
			return err
		}
		for route, start := range shadowSince(revisions) {
			since[route] = start
		}
	}
	return nil
}

func finishReadResult(ctx context.Context, client *Client, token string, result *ReadResult, observations map[string]map[string]routeModeEntry,
	since map[string]time.Time, options replayOptions) {
	entry := observations[result.PackageID][result.RouteID]
	result.Mode = entry.Configured
	if entry.Host != nil {
		result.Compared, result.Mismatches = entry.Host.ShadowTotal, entry.Host.ShadowMismatch
		result.Errors, result.Skipped = entry.Host.ShadowErrors, entry.Host.ShadowSkipped
	}
	if start, ok := since[result.RouteID]; ok && entry.Configured == "shadow" {
		start := start.UTC()
		result.ShadowSince = &start
		duration := time.Since(start).Truncate(time.Second)
		result.ShadowFor, result.ShadowSeconds = duration.String(), duration.Seconds()
	}
	samples, err := client.mismatches(ctx, token, result.PackageID, result.RouteID)
	if err != nil {
		result.Reasons = append(result.Reasons, "mismatch samples unavailable: "+err.Error())
	}
	for _, sample := range samples {
		if result.ShadowSince != nil && sample.ObservedAt.Before(result.ShadowSince.Add(-time.Second)) {
			continue
		}
		result.StoredSamples++
		if len(result.Samples) < 5 {
			digest := SampleDigest{SampleID: sample.SampleID, Path: sample.Path, LegacyStatus: sample.LegacyStatus,
				NativeStatus: sample.NativeStatus, RequestID: sample.RequestID, ObservedAt: sample.ObservedAt}
			for _, diff := range sample.Diff {
				digest.DiffPaths = append(digest.DiffPaths, diff.Kind+" "+diff.Path)
			}
			result.Samples = append(result.Samples, digest)
		}
	}
	switch {
	case result.Skip != "":
		result.Reasons = append(result.Reasons, "not rehearsed: "+result.Skip)
	default:
		if result.Mode != "shadow" {
			result.Reasons = append(result.Reasons, fmt.Sprintf("route is %q, not shadow", result.Mode))
		}
		if result.Compared < uint64(options.MinRequests) { // #nosec G115 -- --min-requests is a small positive flag value
			result.Reasons = append(result.Reasons, fmt.Sprintf("%d compared requests < %d", result.Compared, options.MinRequests))
		}
		if result.Mismatches > 0 || result.StoredSamples > 0 {
			result.Reasons = append(result.Reasons, fmt.Sprintf("%d mismatches (%d samples stored)", result.Mismatches, result.StoredSamples))
		}
		if result.Errors > 0 {
			result.Reasons = append(result.Reasons, fmt.Sprintf("%d native errors in shadow", result.Errors))
		}
		if result.ShadowSince == nil || result.ShadowSeconds < options.MinDuration.Seconds() {
			result.Reasons = append(result.Reasons, fmt.Sprintf("shadow for %s < %s", orNone(result.ShadowFor), options.MinDuration))
		}
		if result.TransportErrors > 0 {
			result.Reasons = append(result.Reasons, fmt.Sprintf("%d requests without an answer", result.TransportErrors))
		}
	}
	result.Pass = len(result.Reasons) == 0
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

// personaTokens logs every persona in. A persona whose login is refused (a
// banned or expired member) gets a token signed with the staging JWT secret
// instead, as if it had logged in before it was banned or expired; without
// the secret its variants are dropped. Both are reported.
func personaTokens(ctx context.Context, client *Client, world *World, common commonFlags) (map[string]string, []string) {
	tokens := map[string]string{}
	var unavailable []string
	keys := make([]string, 0, len(world.Personas))
	for key := range world.Personas {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		token, err := loginPersona(ctx, client, world, common, key)
		if err != nil {
			secret := os.Getenv("STAGING_JWT_SECRET")
			persona := world.Personas[key]
			if secret == "" {
				unavailable = append(unavailable, key+": "+firstLine(err.Error()))
				continue
			}
			if token, err = utils.GenerateToken(persona.ID, persona.Email, false, secret, 7*24*3600); err != nil {
				unavailable = append(unavailable, key+": "+err.Error())
				continue
			}
			unavailable = append(unavailable, key+": login refused, token minted")
		}
		tokens[key] = token
	}
	return tokens, unavailable
}

func loginPersona(ctx context.Context, client *Client, world *World, common commonFlags, key string) (string, error) {
	persona, ok := world.Personas[key]
	if !ok {
		return "", fmt.Errorf("unknown persona %q", key)
	}
	password := persona.Password
	if key == Admin {
		password = common.adminPassword
	}
	return client.Login(ctx, persona.Email, password)
}

// sendAs sends a request as its persona, logging in again once after a 401.
func sendAs(ctx context.Context, client *Client, world *World, common commonFlags, tokens map[string]string, request Req, method string) (Response, error) {
	token := tokens[request.Persona]
	response, err := client.do(ctx, method, request.Path, request.Query, request.BodyBytes(), token)
	if err == nil && response.Status == http.StatusUnauthorized && request.Persona != Anon {
		client.Forget(world.Personas[request.Persona].Email)
		if fresh, loginErr := loginPersona(ctx, client, world, common, request.Persona); loginErr == nil && fresh != token {
			tokens[request.Persona] = fresh
			return client.do(ctx, method, request.Path, request.Query, request.BodyBytes(), fresh)
		}
	}
	return response, err
}

func firstLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return value[:index]
	}
	return value
}

func batchDir(common commonFlags) string {
	return filepath.Join(common.dir, fmt.Sprintf("batch-%d", common.batch))
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", path)
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
