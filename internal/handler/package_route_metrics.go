package handler

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
)

// packageHostHealthDetails is the Health details document written by
// pkg/pluginhostsdk.Router.
type packageHostHealthDetails struct {
	Config struct {
		Status string `json:"status"`
	} `json:"config"`
	Routes map[string]struct {
		Mode         string `json:"mode"`
		Effective    string `json:"effective"`
		Native       uint64 `json:"native_total"`
		NativeErrors uint64 `json:"native_errors"`
	} `json:"routes"`
}

// writePackageRouteMetrics exports the route modes and native counters
// that package hosts report in their Health details. Counters restart with the
// host process, which Prometheus treats as a counter reset.
func writePackageRouteMetrics(body *strings.Builder, stats []pluginhost.HostStats) {
	type routeSample struct {
		pkg, route, mode, effective string
		native, nativeErrors        uint64
	}
	var routes []routeSample
	configStatus := map[string]string{}
	for _, entry := range stats {
		if entry.HealthDetailsJSON == "" {
			continue
		}
		var details packageHostHealthDetails
		if err := json.Unmarshal([]byte(entry.HealthDetailsJSON), &details); err != nil {
			continue
		}
		if details.Config.Status != "" {
			configStatus[entry.PackageID] = details.Config.Status
		}
		for route, detail := range details.Routes {
			routes = append(routes, routeSample{
				pkg: entry.PackageID, route: route, mode: detail.Mode, effective: detail.Effective,
				native: detail.Native, nativeErrors: detail.NativeErrors,
			})
		}
	}
	if len(routes) == 0 && len(configStatus) == 0 {
		return
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].pkg != routes[j].pkg {
			return routes[i].pkg < routes[j].pkg
		}
		return routes[i].route < routes[j].route
	})
	packages := make([]string, 0, len(configStatus))
	for id := range configStatus {
		packages = append(packages, id)
	}
	sort.Strings(packages)
	body.WriteString("# HELP anixops_package_config_status Route-mode configuration status reported by each package host (1 for the current status).\n")
	body.WriteString("# TYPE anixops_package_config_status gauge\n")
	for _, id := range packages {
		body.WriteString("anixops_package_config_status{package=\"" + prometheusLabelValue(id) + "\",status=\"" + prometheusLabelValue(configStatus[id]) + "\"} 1\n")
	}
	labels := func(sample routeSample) string {
		return "package=\"" + prometheusLabelValue(sample.pkg) + "\",route=\"" + prometheusLabelValue(sample.route) + "\""
	}
	body.WriteString("# HELP anixops_package_route_mode Configured and effective mode of package routes that are not legacy or have native traffic.\n")
	body.WriteString("# TYPE anixops_package_route_mode gauge\n")
	for _, sample := range routes {
		body.WriteString("anixops_package_route_mode{" + labels(sample) + ",mode=\"" + prometheusLabelValue(sample.mode) + "\",effective=\"" + prometheusLabelValue(sample.effective) + "\"} 1\n")
	}
	counters := []struct {
		name, help string
		value      func(routeSample) uint64
	}{
		{"anixops_package_native_requests_total", "Requests answered by a package-native route.", func(s routeSample) uint64 { return s.native }},
		{"anixops_package_native_errors_total", "Package-native route requests that failed.", func(s routeSample) uint64 { return s.nativeErrors }},
	}
	for _, counter := range counters {
		body.WriteString("# HELP " + counter.name + " " + counter.help + "\n")
		body.WriteString("# TYPE " + counter.name + " counter\n")
		for _, sample := range routes {
			body.WriteString(counter.name + "{" + labels(sample) + "} " + formatUint(counter.value(sample)) + "\n")
		}
	}
}
