package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/kms9/dars/internal/daemonws"
	"github.com/kms9/dars/internal/realtime"
)

type RegistryOptions struct {
	Realtime *realtime.Metrics
	DaemonWS *daemonws.Metrics
	Version  string
	Commit   string
}

type Registry struct {
	Gatherer prometheus.Gatherer
	HTTP     *HTTPMetrics
	Gateway  *MCPGatewayMetrics
}

// NewRuntimeRegistry builds the Lightweight metrics surface. It deliberately
// excludes database sampling, business rollups and integration-specific
// collectors; the optional listener exposes only process, HTTP and realtime
// health signals.
func NewRuntimeRegistry(opts RegistryOptions) *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dars_build_info",
		Help: "Build information for the DARS server binary.",
	}, []string{"version", "commit"})
	buildInfo.WithLabelValues(defaultLabel(opts.Version, "dev"), defaultLabel(opts.Commit, "unknown")).Set(1)
	reg.MustRegister(buildInfo)

	httpMetrics := NewHTTPMetrics()
	reg.MustRegister(httpMetrics.Collectors()...)
	gatewayMetrics := NewMCPGatewayMetrics()
	reg.MustRegister(gatewayMetrics.Collectors()...)
	if opts.Realtime != nil {
		reg.MustRegister(NewRealtimeCollector(opts.Realtime))
	}
	if opts.DaemonWS != nil {
		reg.MustRegister(NewDaemonWSCollector(opts.DaemonWS))
	}

	return &Registry{
		Gatherer: reg,
		HTTP:     httpMetrics,
		Gateway:  gatewayMetrics,
	}
}

func defaultLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
