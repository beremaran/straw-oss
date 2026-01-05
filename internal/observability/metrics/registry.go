package metrics

import (
	"net/http"
	"net/http/pprof"
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

// RegisterPprof adds pprof handlers to the given mux.
func RegisterPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
}
