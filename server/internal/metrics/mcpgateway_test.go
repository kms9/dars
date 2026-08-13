package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMCPGatewayMetricsBoundLabelsAndExcludeSensitiveValues(t *testing.T) {
	const sentinel = "Bearer dat_secret-upstream-credential"
	metrics := NewMCPGatewayMetrics()
	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics.Collectors()...)

	metrics.RecordInvocation(sentinel, sentinel, time.Second, 123, 456)
	metrics.RecordSourceValidation(sentinel, sentinel)
	metrics.RecordBundlePublication(sentinel, 7)
	metrics.RecordInvocation("openapi", "succeeded", time.Millisecond, 1, 2)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather gateway metrics: %v", err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if strings.Contains(label.GetValue(), sentinel) {
					t.Fatalf("metric %q leaked sensitive label value", family.GetName())
				}
			}
		}
	}

	assertMetricHasLabels(t, families, "dars_mcp_gateway_tool_invocations_total", map[string]string{
		"source_kind": "invalid", "outcome": "other",
	})
	assertMetricHasLabels(t, families, "dars_mcp_gateway_source_validations_total", map[string]string{
		"source_kind": "invalid", "outcome": "other",
	})
	assertMetricHasLabels(t, families, "dars_mcp_gateway_bundle_publications_total", map[string]string{
		"outcome": "other",
	})
}

func assertMetricHasLabels(t *testing.T, families []*dto.MetricFamily, name string, expected map[string]string) {
	t.Helper()
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			actual := make(map[string]string, len(metric.Label))
			for _, label := range metric.Label {
				actual[label.GetName()] = label.GetValue()
			}
			matches := true
			for key, value := range expected {
				if actual[key] != value {
					matches = false
					break
				}
			}
			if matches {
				return
			}
		}
	}
	t.Fatalf("metric %q does not contain labels %v", name, expected)
}
