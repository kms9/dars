package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MCPGatewayMetrics contains only bounded labels. Workspace, Agent, Task,
// Bundle, Source and tool identities remain in the redacted activity audit.
type MCPGatewayMetrics struct {
	invocations      *prometheus.CounterVec
	duration         *prometheus.HistogramVec
	requestBytes     *prometheus.HistogramVec
	responseBytes    *prometheus.HistogramVec
	sourceValidation *prometheus.CounterVec
	bundlePublish    *prometheus.CounterVec
	bundleItems      prometheus.Histogram
}

func NewMCPGatewayMetrics() *MCPGatewayMetrics {
	return &MCPGatewayMetrics{
		invocations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dars", Subsystem: "mcp_gateway", Name: "tool_invocations_total",
			Help: "Total MCP Gateway tool invocations by Source kind and bounded outcome.",
		}, []string{"source_kind", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dars", Subsystem: "mcp_gateway", Name: "tool_invocation_duration_seconds",
			Help: "MCP Gateway tool invocation latency.", Buckets: prometheus.DefBuckets,
		}, []string{"source_kind", "outcome"}),
		requestBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dars", Subsystem: "mcp_gateway", Name: "tool_request_bytes",
			Help: "Validated MCP tool argument bytes.", Buckets: prometheus.ExponentialBuckets(64, 4, 9),
		}, []string{"source_kind"}),
		responseBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dars", Subsystem: "mcp_gateway", Name: "tool_response_bytes",
			Help: "MCP tool response bytes.", Buckets: prometheus.ExponentialBuckets(64, 4, 10),
		}, []string{"source_kind"}),
		sourceValidation: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dars", Subsystem: "mcp_gateway", Name: "source_validations_total",
			Help: "Total Tool Source validation outcomes.",
		}, []string{"source_kind", "outcome"}),
		bundlePublish: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dars", Subsystem: "mcp_gateway", Name: "bundle_publications_total",
			Help: "Total ToolBundle publication outcomes.",
		}, []string{"outcome"}),
		bundleItems: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "dars", Subsystem: "mcp_gateway", Name: "bundle_item_count",
			Help: "Number of immutable items in a published ToolBundle.", Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128, 256},
		}),
	}
}

func (metrics *MCPGatewayMetrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		metrics.invocations, metrics.duration, metrics.requestBytes, metrics.responseBytes,
		metrics.sourceValidation, metrics.bundlePublish, metrics.bundleItems,
	}
}

func (metrics *MCPGatewayMetrics) RecordInvocation(sourceKind, outcome string, elapsed time.Duration, inputBytes, outputBytes int) {
	if metrics == nil {
		return
	}
	sourceKind = boundedSourceKind(sourceKind)
	outcome = boundedInvocationOutcome(outcome)
	metrics.invocations.WithLabelValues(sourceKind, outcome).Inc()
	metrics.duration.WithLabelValues(sourceKind, outcome).Observe(elapsed.Seconds())
	metrics.requestBytes.WithLabelValues(sourceKind).Observe(float64(inputBytes))
	metrics.responseBytes.WithLabelValues(sourceKind).Observe(float64(outputBytes))
}

func (metrics *MCPGatewayMetrics) RecordSourceValidation(sourceKind, outcome string) {
	if metrics != nil {
		metrics.sourceValidation.WithLabelValues(boundedSourceKind(sourceKind), boundedValidationOutcome(outcome)).Inc()
	}
}

func (metrics *MCPGatewayMetrics) RecordBundlePublication(outcome string, itemCount int) {
	if metrics == nil {
		return
	}
	switch outcome {
	case "created", "no_op", "cleared", "revoked", "failed":
	default:
		outcome = "other"
	}
	metrics.bundlePublish.WithLabelValues(outcome).Inc()
	if outcome == "created" || outcome == "no_op" {
		metrics.bundleItems.Observe(float64(itemCount))
	}
}

func boundedSourceKind(value string) string {
	switch value {
	case "server_local", "remote_mcp", "grpc", "openapi":
		return value
	default:
		return "invalid"
	}
}

func boundedInvocationOutcome(value string) string {
	switch value {
	case "succeeded", "failed", "invalid_argument", "limited_or_cancelled", "cancelled_or_timed_out", "response_too_large":
		return value
	default:
		return "other"
	}
}

func boundedValidationOutcome(value string) string {
	switch value {
	case "ready", "tool_source_invalid", "tool_source_unreachable", "tool_source_validator_unavailable", "egress_forbidden":
		return value
	default:
		return "other"
	}
}
