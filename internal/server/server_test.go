package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/timo-reymann/duplicati-exporter/internal/buildinfo"
	"github.com/timo-reymann/duplicati-exporter/internal/collector"
	"github.com/timo-reymann/duplicati-exporter/internal/duplicati"
	"github.com/timo-reymann/duplicati-exporter/internal/webhook"
)

// fakeDuplicati serves the minimal API surface the exporter needs.
type fakeDuplicati struct {
	server  *httptest.Server
	failing bool
	backups []duplicati.BackupWithSchedule
}

func newFakeDuplicati(t *testing.T) *fakeDuplicati {
	t.Helper()
	f := &fakeDuplicati{
		backups: []duplicati.BackupWithSchedule{{
			Backup: duplicati.Backup{
				ID:            "b1",
				Name:          "photos",
				OperationType: "Backup",
				Metadata:      map[string]string{"LastBackupFinished": time.Now().UTC().Format("20060102T150405Z")},
				Settings:      []duplicati.Setting{{Name: "--machine-id", Value: "m-1"}},
			},
			Schedule: &duplicati.Schedule{ID: 1, Time: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Repeat: "1D"},
		}},
	}

	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if f.failing {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"AccessToken": "tok"})
	})
	mux.HandleFunc("/api/v1/systeminfo", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(duplicati.SystemInfo{
			ServerVersionName: "2.1.0.5_beta",
			ServerTimeZone:    "UTC",
			MachineName:       "test-host",
		})
	}))
	mux.HandleFunc("/api/v1/serverstate", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(duplicati.ServerState{ProgramState: "Running"})
	}))
	mux.HandleFunc("/api/v1/backups", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.backups)
	}))
	mux.HandleFunc("/api/v1/notifications", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]duplicati.Notification{})
	}))
	mux.HandleFunc("/api/v1/progressstate", auth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func newTestServer(t *testing.T, api *fakeDuplicati) (*Server, *webhook.Store) {
	t.Helper()
	u, err := url.Parse(api.server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	client := duplicati.NewClient(u, "secret", "", false, 5*time.Second)
	info, err := duplicati.Discover(context.Background(), client)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	store := webhook.NewStore()
	exporter := collector.New(collector.Options{
		Targets:         []*collector.Target{{Name: "test", Client: client, Machine: info}},
		Webhooks:        store,
		Timeout:         5 * time.Second,
		FilesetsEnabled: false,
		Build:           buildinfo.Info{Version: "test"},
	})

	return New(Config{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		ReportPath:    "/report",
		Version:       "test",
	}, exporter, store), store
}

func TestMetricsEndpoint(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"duplicati_machine_up{machine_id=", "duplicati_backup_stale{",
		"duplicati_machine_scrape_error{",
		"duplicati_build_info{",
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}

func TestMetricsEndpointReturns500WhenAllMachinesDown(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)
	api.failing = true

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestMetricsRejectsPost(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)

	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestReportEndpointAcceptsJSON(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, store := newTestServer(t, api)

	body := `{"Data":{"MainOperation":"Backup","ParsedResult":"Success"},
	          "Extra":{"machine-id":"m-1","backup-name":"photos"}}`
	req := httptest.NewRequest(http.MethodPost, "/report", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if store.Len() != 1 {
		t.Errorf("store.Len() = %d, want 1", store.Len())
	}
	r, ok := store.Lookup("m-1", "", "photos")
	if !ok || r.ParsedResult != "Success" {
		t.Fatalf("stored report = %+v, ok=%v", r, ok)
	}
}

func TestReportEndpointRejectsUnknownContentType(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, store := newTestServer(t, api)

	req := httptest.NewRequest(http.MethodPost, "/report", strings.NewReader("<x/>"))
	req.Header.Set("Content-Type", "application/xml")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
	if store.Len() != 0 {
		t.Errorf("store.Len() = %d, want 0", store.Len())
	}
}

func TestReportEndpointRejectsGet(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)

	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestReportIsVisibleInMetrics(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)

	body := `{"Data":{"MainOperation":"Backup","ParsedResult":"Fatal",
	          "BackendStatistics":{"BytesUploaded":4096,"FilesUploaded":2}},
	          "Extra":{"machine-id":"m-1","backup-name":"photos"}}`
	req := httptest.NewRequest(http.MethodPost, "/report", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("report status = %d, want 202", rec.Code)
	}

	get := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	out := httptest.NewRecorder()
	srv.Handler().ServeHTTP(out, get)

	payload := out.Body.String()
	for _, want := range []string{
		`duplicati_backup_last_result{backup_name="photos",machine_id="m-1",machine_name="test-host"} 2`,
		`duplicati_backup_bytes_uploaded_total{backup_name="photos",machine_id="m-1",machine_name="test-host"} 4096`,
	} {
		if !strings.Contains(payload, want) {
			t.Errorf("metrics missing %q\n%s", want, payload)
		}
	}
}

func TestHealthEndpoints(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)

	for _, path := range []string{"/-/healthy", "/-/ready", "/healthz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, rec.Code)
		}
	}
}

func TestIndexPage(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "duplicati-exporter") {
		t.Error("index page does not mention the exporter")
	}
	if !strings.Contains(rec.Body.String(), "/metrics") {
		t.Error("index page does not link the metrics path")
	}
}

func TestUnknownPathIs404(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestListenAndServeStopsOnContextCancel(t *testing.T) {
	api := newFakeDuplicati(t)
	srv, _ := newTestServer(t, api)
	srv.cfg.ListenAddress = "127.0.0.1:0"

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe(ctx) }()

	// Give the server a moment to bind, then stop it.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListenAndServe() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe() did not stop after context cancellation")
	}
}
