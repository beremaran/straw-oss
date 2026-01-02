package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry *prometheus.Registry
	once     sync.Once
)

// Init initializes the global metrics registry.
func Init() *prometheus.Registry {
	once.Do(func() {
		registry = prometheus.NewRegistry()

		// Register standard Go runtime metrics
		registry.MustRegister(collectors.NewGoCollector())
		registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	})
	return registry
}

// GetRegistry returns the global metrics registry.
func GetRegistry() *prometheus.Registry {
	if registry == nil {
		return Init()
	}
	return registry
}

// Handler returns the HTTP handler for Prometheus metrics.
func Handler() http.Handler {
	return promhttp.HandlerFor(GetRegistry(), promhttp.HandlerOpts{
		Registry: GetRegistry(),
	})
}
