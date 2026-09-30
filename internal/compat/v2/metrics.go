package v2

import (
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Gateway error codes. They are the complete, fixed set of values for the
// "code" label of anixops_v2_gateway_errors_total.
const (
	codePackageUnavailable           = "package_unavailable"
	codeRouteNotFound                = "package_route_not_found"
	codeRouteRequiresWebSocket       = "package_route_requires_websocket"
	codeRouteRequiresHTTP            = "package_route_requires_http"
	codePluginHostUnavailable        = "plugin_host_unavailable"
	codePluginHostIncompatible       = "plugin_host_incompatible"
	codePluginRequestTooLarge        = "plugin_request_too_large"
	codePluginResponseTooLarge       = "plugin_response_too_large"
	codePluginRequestInvalid         = "plugin_request_invalid"
	codePackageRouteFrozen           = "package_route_frozen"
	metricsUnresolvedPackage         = "unresolved"
	metricsUnresolvedRoute           = "unresolved"
	metricsOverflowLabel             = "_overflow"
	defaultGatewayMetricsSeriesLimit = 4096
)

// gatewayLatencyBuckets are the fixed upper bounds, in seconds, of the
// anixops_v2_gateway_request_duration_seconds histogram.
var gatewayLatencyBuckets = []float64{0.005, 0.025, 0.1, 0.25, 1, 2.5, 10}

// GatewayMetrics is a small concurrency-safe Prometheus registry for the v2
// compatibility gateways. Label values are bounded: package and route come
// from verified package route declarations (never the raw request path),
// unresolved requests share one "unresolved" series, error codes are a fixed
// set, and a hard series cap folds any excess into "_overflow".
type GatewayMetrics struct {
	mu          sync.Mutex
	seriesLimit int
	requests    map[requestSeries]uint64
	errors      map[errorSeries]uint64
	latency     map[string]*latencyHistogram
}

type requestSeries struct {
	packageID string
	route     string
	codeClass string
}

type errorSeries struct {
	packageID string
	route     string
	code      string
}

type latencyHistogram struct {
	buckets []uint64
	count   uint64
	sum     float64
}

var defaultGatewayMetrics = NewGatewayMetrics()

// DefaultGatewayMetrics returns the process-wide registry rendered by
// /metrics. Gateways use it unless given their own registry.
func DefaultGatewayMetrics() *GatewayMetrics { return defaultGatewayMetrics }

// NewGatewayMetrics returns an empty registry.
func NewGatewayMetrics() *GatewayMetrics {
	return &GatewayMetrics{
		seriesLimit: defaultGatewayMetricsSeriesLimit,
		requests:    make(map[requestSeries]uint64),
		errors:      make(map[errorSeries]uint64),
		latency:     make(map[string]*latencyHistogram),
	}
}

// Observe records one completed gateway request. errorCode is empty for a
// request the package host answered.
func (m *GatewayMetrics) Observe(packageID, routeID string, status int, errorCode string, duration time.Duration) {
	if m == nil {
		return
	}
	if packageID == "" || routeID == "" {
		packageID, routeID = metricsUnresolvedPackage, metricsUnresolvedRoute
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	request := requestSeries{packageID: packageID, route: routeID, codeClass: statusClass(status)}
	if _, exists := m.requests[request]; !exists && len(m.requests) >= m.seriesLimit {
		request.packageID, request.route = metricsOverflowLabel, metricsOverflowLabel
	}
	m.requests[request]++
	if errorCode != "" {
		series := errorSeries{packageID: packageID, route: routeID, code: errorCode}
		if _, exists := m.errors[series]; !exists && len(m.errors) >= m.seriesLimit {
			series.packageID, series.route = metricsOverflowLabel, metricsOverflowLabel
		}
		m.errors[series]++
	}
	histogramKey := packageID
	histogram := m.latency[histogramKey]
	if histogram == nil {
		if len(m.latency) >= m.seriesLimit {
			histogramKey = metricsOverflowLabel
			histogram = m.latency[histogramKey]
		}
		if histogram == nil {
			histogram = &latencyHistogram{buckets: make([]uint64, len(gatewayLatencyBuckets))}
			m.latency[histogramKey] = histogram
		}
	}
	for index, bound := range gatewayLatencyBuckets {
		if seconds <= bound {
			histogram.buckets[index]++
		}
	}
	histogram.count++
	histogram.sum += seconds
}

// WritePrometheus renders the registry in the Prometheus text format.
func (m *GatewayMetrics) WritePrometheus(writer io.Writer) error {
	if m == nil {
		return nil
	}
	var builder strings.Builder
	m.mu.Lock()
	requests := make([]requestSeries, 0, len(m.requests))
	for series := range m.requests {
		requests = append(requests, series)
	}
	sort.Slice(requests, func(i, j int) bool {
		a, b := requests[i], requests[j]
		if a.packageID != b.packageID {
			return a.packageID < b.packageID
		}
		if a.route != b.route {
			return a.route < b.route
		}
		return a.codeClass < b.codeClass
	})
	builder.WriteString("# HELP anixops_v2_gateway_requests_total Requests handled by the v2 package gateways.\n")
	builder.WriteString("# TYPE anixops_v2_gateway_requests_total counter\n")
	for _, series := range requests {
		builder.WriteString("anixops_v2_gateway_requests_total{package=\"" + escapeLabel(series.packageID) +
			"\",route=\"" + escapeLabel(series.route) + "\",code_class=\"" + series.codeClass + "\"} " +
			strconv.FormatUint(m.requests[series], 10) + "\n")
	}

	failures := make([]errorSeries, 0, len(m.errors))
	for series := range m.errors {
		failures = append(failures, series)
	}
	sort.Slice(failures, func(i, j int) bool {
		a, b := failures[i], failures[j]
		if a.packageID != b.packageID {
			return a.packageID < b.packageID
		}
		if a.route != b.route {
			return a.route < b.route
		}
		return a.code < b.code
	})
	builder.WriteString("# HELP anixops_v2_gateway_errors_total Gateway-generated error responses by error code.\n")
	builder.WriteString("# TYPE anixops_v2_gateway_errors_total counter\n")
	for _, series := range failures {
		builder.WriteString("anixops_v2_gateway_errors_total{package=\"" + escapeLabel(series.packageID) +
			"\",route=\"" + escapeLabel(series.route) + "\",code=\"" + escapeLabel(series.code) + "\"} " +
			strconv.FormatUint(m.errors[series], 10) + "\n")
	}

	packages := make([]string, 0, len(m.latency))
	for packageID := range m.latency {
		packages = append(packages, packageID)
	}
	sort.Strings(packages)
	builder.WriteString("# HELP anixops_v2_gateway_request_duration_seconds Time from gateway entry to response, by package.\n")
	builder.WriteString("# TYPE anixops_v2_gateway_request_duration_seconds histogram\n")
	for _, packageID := range packages {
		histogram := m.latency[packageID]
		label := "package=\"" + escapeLabel(packageID) + "\""
		for index, bound := range gatewayLatencyBuckets {
			builder.WriteString("anixops_v2_gateway_request_duration_seconds_bucket{" + label + ",le=\"" +
				strconv.FormatFloat(bound, 'g', -1, 64) + "\"} " + strconv.FormatUint(histogram.buckets[index], 10) + "\n")
		}
		builder.WriteString("anixops_v2_gateway_request_duration_seconds_bucket{" + label + ",le=\"+Inf\"} " + strconv.FormatUint(histogram.count, 10) + "\n")
		builder.WriteString("anixops_v2_gateway_request_duration_seconds_sum{" + label + "} " + strconv.FormatFloat(histogram.sum, 'g', -1, 64) + "\n")
		builder.WriteString("anixops_v2_gateway_request_duration_seconds_count{" + label + "} " + strconv.FormatUint(histogram.count, 10) + "\n")
	}
	m.mu.Unlock()
	_, err := io.WriteString(writer, builder.String())
	return err
}

func statusClass(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return strconv.Itoa(status/100) + "xx"
}

func escapeLabel(value string) string {
	if !strings.ContainsAny(value, "\\\"\n") {
		return value
	}
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return strings.ReplaceAll(value, "\n", "\\n")
}
