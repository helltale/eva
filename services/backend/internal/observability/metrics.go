package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Approximate in-flight HTTP handlers (live sampling).",
	})

	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by method, path pattern, status.",
	}, []string{"method", "path", "code"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	LLMRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_requests_total",
		Help: "LLM API calls",
	}, []string{"result"})

	LLMDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "llm_request_duration_seconds",
		Help:    "LLM request latency",
		Buckets: []float64{.1, .25, .5, 1, 2, 5, 10, 30, 60, 120},
	})

	ToolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tool_calls_total",
		Help: "Tool invocations",
	}, []string{"tool", "result"})

	SearchRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_requests_total",
		Help: "Search requests",
	}, []string{"result"})

	WSSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ws_active_sessions",
		Help: "Active WebSocket realtime sessions",
	})

	TTSRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tts_requests_total",
		Help: "TTS synthesize requests",
	})

	TTSDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "tts_stream_duration_seconds",
		Help:    "TTS streaming duration",
		Buckets: prometheus.DefBuckets,
	})

	STTRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stt_requests_total",
		Help: "STT sessions",
	})

	VoiceInterrupts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "voice_interrupts_total",
		Help: "Barge-in / interrupt events",
	})
)

func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// MetricsMiddleware records request counts and latency. Path label is chi.RoutePattern or URL path.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		path := r.URL.Path
		code := strconv.Itoa(rw.status)
		httpRequests.WithLabelValues(r.Method, path, code).Inc()
		httpDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
