// Package server wires the exporter's HTTP endpoints.
package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/timo-reymann/duplicati-exporter/internal/collector"
	"github.com/timo-reymann/duplicati-exporter/internal/webhook"
)

// Config describes the HTTP surface of the exporter.
type Config struct {
	ListenAddress string
	MetricsPath   string
	ReportPath    string
	Logger        *slog.Logger
	Version       string
}

// Server serves /metrics, health probes and the Duplicati webhook.
type Server struct {
	cfg      Config
	logger   *slog.Logger
	exporter *collector.Exporter
	store    *webhook.Store
	registry *prometheus.Registry
	mux      *http.ServeMux
	ready    atomic.Bool
}

// New builds the HTTP server. The exporter is polled lazily on every scrape.
func New(cfg Config, exporter *collector.Exporter, store *webhook.Store) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MetricsPath == "" {
		cfg.MetricsPath = "/metrics"
	}
	if cfg.ReportPath == "" {
		cfg.ReportPath = "/report"
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter)
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	s := &Server{
		cfg:      cfg,
		logger:   cfg.Logger,
		exporter: exporter,
		store:    store,
		registry: registry,
		mux:      http.NewServeMux(),
	}
	s.routes()
	s.ready.Store(true)
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc(s.cfg.MetricsPath, s.handleMetrics)
	s.mux.HandleFunc(s.cfg.ReportPath, s.handleReport)
	s.mux.HandleFunc("/-/healthy", s.handleHealthy)
	s.mux.HandleFunc("/-/ready", s.handleHealthy)
	s.mux.HandleFunc("/healthz", s.handleHealthy)
	s.mux.HandleFunc("/", s.handleIndex)
}

// Handler returns the instrumented HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.recoverer(s.logRequests(s.mux))
}

// ListenAndServe runs the HTTP server until ctx is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              s.cfg.ListenAddress,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	s.logger.Info("listening",
		slog.String("address", s.cfg.ListenAddress),
		slog.String("metrics_path", s.cfg.MetricsPath),
		slog.String("report_path", s.cfg.ReportPath))

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// handleMetrics polls every Duplicati machine and then renders the metrics. It
// only fails the whole scrape when every machine is unreachable.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	if err := s.exporter.Poll(r.Context()); err != nil {
		if errors.Is(err, collector.ErrAllMachinesDown) {
			s.logger.Error("scrape failed", slog.Any("err", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.logger.Error("scrape failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Debug("scrape completed", slog.Duration("took", time.Since(start)))

	promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{
		ErrorHandling:       promhttp.ContinueOnError,
		MaxRequestsInFlight: 4,
		Registry:            s.registry,
	}).ServeHTTP(w, r)
}

// handleReport accepts a Duplicati send-http report.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost, http.MethodPut:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := webhook.Decode(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	report, err := webhook.ParseBody(r.Header.Get("Content-Type"), body)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, webhook.ErrUnsupported) {
			status = http.StatusUnsupportedMediaType
		}
		s.logger.Warn("rejected webhook report",
			slog.String("remote", r.RemoteAddr), slog.Any("err", err))
		http.Error(w, err.Error(), status)
		return
	}

	s.store.Put(report)
	s.logger.Debug("stored webhook report",
		slog.String("machine_id", report.MachineID),
		slog.String("machine_name", report.MachineName),
		slog.String("backup", report.BackupName),
		slog.String("result", report.ParsedResult))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = fmt.Fprint(w, `{"status":"ok"}`+"\n")
}

func (s *Server) handleHealthy(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "OK\n")
}

var indexTmpl = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>duplicati-exporter</title>
<style>
 body { font-family: system-ui, sans-serif; margin: 3rem auto; max-width: 60rem; line-height: 1.5; }
 code { background: #f2f2f2; padding: .1rem .3rem; border-radius: 3px; }
 ul { padding-left: 1.2rem; }
</style>
</head>
<body>
<h1>duplicati-exporter</h1>
<p>Version <code>{{.Version}}</code></p>
<h2>Endpoints</h2>
<ul>
 <li><a href="{{.MetricsPath}}">{{.MetricsPath}}</a> &mdash; Prometheus metrics</li>
 <li><a href="/-/healthy">/-/healthy</a> &mdash; liveness probe</li>
 <li><a href="/-/ready">/-/ready</a> &mdash; readiness probe</li>
</ul>
<h2>Duplicati integration</h2>
<p>Point Duplicati at the exporter to enrich metrics with per-run transfer
statistics:</p>
<pre><code>--send-http-url={{.SelfURL}}{{.ReportPath}}
--send-http-result-output-format=Json
--send-http-any-operation=true</code></pre>
<p>See the <code>README.md</code> for the full metric catalog.</p>
</body>
</html>`))

type indexData struct {
	Version     string
	MetricsPath string
	ReportPath  string
	SelfURL     string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	self := "http://" + r.Host
	if r.TLS != nil {
		self = "https://" + r.Host
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTmpl.Execute(w, indexData{
		Version:     s.cfg.Version,
		MetricsPath: s.cfg.MetricsPath,
		ReportPath:  s.cfg.ReportPath,
		SelfURL:     self,
	}); err != nil {
		s.logger.Error("render index", slog.Any("err", err))
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == s.cfg.MetricsPath || r.URL.Path == s.cfg.ReportPath {
			s.logger.Debug("request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("took", time.Since(start)),
				slog.String("remote", r.RemoteAddr))
		}
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic in handler",
					slog.Any("panic", rec),
					slog.String("path", r.URL.Path))
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
