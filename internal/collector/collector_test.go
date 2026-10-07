package collector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/timo-reymann/duplicati-exporter/internal/buildinfo"
	"github.com/timo-reymann/duplicati-exporter/internal/duplicati"
	"github.com/timo-reymann/duplicati-exporter/internal/webhook"
)

// fakeAPI implements just enough of the Duplicati API to exercise the collector.
type fakeAPI struct {
	server *httptest.Server

	machineName   string
	machineID     string
	programState  string
	backups       []duplicati.BackupWithSchedule
	notifications []duplicati.Notification
	filesets      map[string][]duplicati.Fileset
	fail          atomic.Bool
	notifyFail    atomic.Bool
	filesetCalls  int32
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{
		machineName:  "test-host",
		machineID:    "m-1",
		programState: "Running",
		filesets:     map[string][]duplicati.Fileset{},
	}

	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if f.fail.Load() {
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

	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"AccessToken": "tok"})
	})
	mux.HandleFunc("/api/v1/systeminfo", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(duplicati.SystemInfo{
			ServerVersionName: "2.1.0.5_beta",
			ServerTimeZone:    "UTC",
			MachineName:       f.machineName,
			OSType:            "Linux",
		})
	}))
	mux.HandleFunc("/api/v1/serverstate", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(duplicati.ServerState{ProgramState: f.programState})
	}))
	mux.HandleFunc("/api/v1/backups", auth(func(w http.ResponseWriter, _ *http.Request) {
		// Duplicati only exposes machine-id as a backup option, so the fake
		// stamps it on every backup like a real deployment would.
		out := make([]duplicati.BackupWithSchedule, len(f.backups))
		copy(out, f.backups)
		for i := range out {
			out[i].Backup.Settings = append(
				[]duplicati.Setting{{Name: "--machine-id", Value: f.machineID}},
				out[i].Backup.Settings...)
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	mux.HandleFunc("/api/v1/notifications", auth(func(w http.ResponseWriter, _ *http.Request) {
		if f.notifyFail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(f.notifications)
	}))
	mux.HandleFunc("/api/v1/progressstate", auth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	mux.HandleFunc("/api/v1/backup/", auth(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.filesetCalls, 1)
		id := r.URL.Path[len("/api/v1/backup/"):]
		if i := indexOf(id, "/filesets"); i >= 0 {
			id = id[:i]
			if fs, ok := f.filesets[id]; ok {
				_ = json.NewEncoder(w).Encode(fs)
				return
			}
			_ = json.NewEncoder(w).Encode([]duplicati.Fileset{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func (f *fakeAPI) target(t *testing.T) *Target {
	t.Helper()
	u, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	client := duplicati.NewClient(u, "secret", "", false, 5*time.Second)
	info, err := duplicati.Discover(context.Background(), client)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	return &Target{Name: "fake", Client: client, Machine: info}
}

func newTestExporter(t *testing.T, targets ...*Target) (*Exporter, *webhook.Store, *prometheus.Registry) {
	t.Helper()
	store := webhook.NewStore()
	exp := New(Options{
		Targets:          targets,
		Webhooks:         store,
		Timeout:          5 * time.Second,
		FilesetsEnabled:  false,
		FilesetsCacheTTL: time.Minute,
		Build:            buildinfo.Info{Version: "test", Revision: "abc", GoVersion: "go1.26"},
	})
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(exp)
	return exp, store, reg
}

func gather(t *testing.T, reg *prometheus.Registry) map[string]*dto.MetricFamily {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	out := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		out[f.GetName()] = f
	}
	return out
}

func metricValue(t *testing.T, families map[string]*dto.MetricFamily, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	f, ok := families[name]
	if !ok {
		return 0, false
	}
	for _, m := range f.GetMetric() {
		if matchLabels(m, labels) {
			switch {
			case m.GetGauge() != nil:
				return m.GetGauge().GetValue(), true
			case m.GetCounter() != nil:
				return m.GetCounter().GetValue(), true
			case m.GetUntyped() != nil:
				return m.GetUntyped().GetValue(), true
			}
		}
	}
	return 0, false
}

func matchLabels(m *dto.Metric, want map[string]string) bool {
	if len(m.GetLabel()) != len(want) {
		return false
	}
	for _, l := range m.GetLabel() {
		if v, ok := want[l.GetName()]; !ok || v != l.GetValue() {
			return false
		}
	}
	return true
}

func expectValue(t *testing.T, families map[string]*dto.MetricFamily, name string, labels map[string]string, want float64) {
	t.Helper()
	got, ok := metricValue(t, families, name, labels)
	if !ok {
		t.Fatalf("metric %s%v not found", name, labels)
	}
	if got != want {
		t.Errorf("metric %s%v = %v, want %v", name, labels, got, want)
	}
}

func mLabels(id, name string) map[string]string {
	return map[string]string{"machine_id": id, "machine_name": name}
}

func bLabels(id, name, backup string) map[string]string {
	return map[string]string{"machine_id": id, "machine_name": name, "backup_name": backup}
}

func healthyBackup(nextRun time.Time, lastFinished, lastDate time.Time) duplicati.BackupWithSchedule {
	meta := map[string]string{
		duplicati.MetaLastBackupStarted:  lastFinished.Add(-5 * time.Minute).Format("20060102T150405Z"),
		duplicati.MetaLastBackupFinished: lastFinished.Format("20060102T150405Z"),
		duplicati.MetaLastBackupDuration: "00:05:00",
		duplicati.MetaLastBackupDate:     lastDate.Format("20060102T150405Z"),
		duplicati.MetaSourceFilesCount:   "1000",
		duplicati.MetaSourceFilesSize:    "12345678",
		duplicati.MetaTargetFilesCount:   "1000",
		duplicati.MetaTargetFilesSize:    "12345678",
		duplicati.MetaBackupListCount:    "7",
		duplicati.MetaTotalQuotaSpace:    "1000000",
		duplicati.MetaFreeQuotaSpace:     "400000",
		duplicati.MetaAssignedQuota:      "1000000",
	}
	return duplicati.BackupWithSchedule{
		Backup: duplicati.Backup{
			ID:            "b1",
			Name:          "photos",
			OperationType: "Backup",
			Metadata:      meta,
			Settings:      []duplicati.Setting{{Name: "--machine-id", Value: "m-1"}},
		},
		Schedule: &duplicati.Schedule{
			ID:      1,
			Time:    nextRun.UTC().Format(time.RFC3339),
			Repeat:  "1D",
			LastRun: lastFinished.UTC().Format(time.RFC3339),
		},
	}
}

func TestCollectHealthyBackup(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}

	exp, _, reg := newTestExporter(t, api.target(t))
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	families := gather(t, reg)
	l := mLabels("m-1", "test-host")
	b := bLabels("m-1", "test-host", "photos")

	expectValue(t, families, "duplicati_machine_up", l, 1)
	expectValue(t, families, "duplicati_machine_scrape_error", l, 0)
	expectValue(t, families, "duplicati_machine_paused", l, 0)
	expectValue(t, families, "duplicati_machine_info", map[string]string{
		"machine_id": "m-1", "machine_name": "test-host",
		"timezone": "UTC", "version": "2.1.0.5_beta", "os_type": "Linux", "os_version": "",
	}, 1)

	expectValue(t, families, "duplicati_backup_stale", b, 0)
	expectValue(t, families, "duplicati_backup_failure", b, 0)
	expectValue(t, families, "duplicati_backup_last_date_time", b, float64(now.Add(-time.Hour).Unix()))
	expectValue(t, families, "duplicati_backup_last_duration_seconds", b, 300)
	expectValue(t, families, "duplicati_backup_source_files_count", b, 1000)
	expectValue(t, families, "duplicati_backup_source_files_bytes", b, 12345678)
	expectValue(t, families, "duplicati_backup_list_count", b, 7)
	expectValue(t, families, "duplicati_backup_total_quota_bytes", b, 1000000)
	expectValue(t, families, "duplicati_backup_next_run_time", b, float64(now.Add(24*time.Hour).Unix()))
	expectValue(t, families, "duplicati_build_info", map[string]string{
		"version": "test", "revision": "abc", "goversion": "go1.26",
	}, 1)
}

func TestCollectStaleWhenSchedulePassedWithoutSuccess(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()

	tests := []struct {
		name       string
		nextRun    time.Time
		lastFinish time.Time
		lastDate   time.Time
		wantStale  float64
	}{
		{
			name: "scheduled run passed, nothing finished",
			// last finished at zero value would be before nextRun
			nextRun:    now.Add(-time.Hour),
			lastFinish: now.Add(-48 * time.Hour),
			lastDate:   now.Add(-48 * time.Hour),
			wantStale:  1,
		},
		{
			name:       "ran and failed: finished after next run but no successful upload",
			nextRun:    now.Add(-24 * time.Hour),
			lastFinish: now.Add(-time.Hour),
			lastDate:   now.Add(-48 * time.Hour),
			wantStale:  1,
		},
		{
			name:       "successful run after next run",
			nextRun:    now.Add(-24 * time.Hour),
			lastFinish: now.Add(-time.Hour),
			lastDate:   now.Add(-time.Hour),
			wantStale:  0,
		},
		{
			name:       "finished after next run, last backup date not reported",
			nextRun:    now.Add(-24 * time.Hour),
			lastFinish: now.Add(-time.Hour),
			lastDate:   time.Time{},
			wantStale:  0,
		},
		{
			name:       "next run still in the future",
			nextRun:    now.Add(time.Hour),
			lastFinish: now.Add(-48 * time.Hour),
			lastDate:   now.Add(-48 * time.Hour),
			wantStale:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api.backups = []duplicati.BackupWithSchedule{healthyBackup(tt.nextRun, tt.lastFinish, tt.lastDate)}
			exp, _, reg := newTestExporter(t, api.target(t))
			if err := exp.Poll(context.Background()); err != nil {
				t.Fatalf("Poll() error = %v", err)
			}
			expectValue(t, gather(t, reg), "duplicati_backup_stale",
				bLabels("m-1", "test-host", "photos"), tt.wantStale)
		})
	}
}

func TestCollectStaleUnscheduledBackupIsNotStale(t *testing.T) {
	api := newFakeAPI(t)
	b := healthyBackup(time.Now().Add(time.Hour), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	b.Schedule = nil
	api.backups = []duplicati.BackupWithSchedule{b}

	exp, _, reg := newTestExporter(t, api.target(t))
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if _, ok := metricValue(t, gather(t, reg), "duplicati_backup_stale", bLabels("m-1", "test-host", "photos")); ok {
		t.Error("stale metric emitted for an unscheduled backup")
	}
}

func TestCollectFailureFromErrorNotification(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}
	api.notifications = []duplicati.Notification{
		{ID: 1, Type: "Error", BackupID: "b1", Message: "boom"},
		{ID: 2, Type: "Warning", BackupID: "b1", Message: "meh"},
		{ID: 3, Type: "Information", BackupID: "other", Message: "info"},
	}

	exp, _, reg := newTestExporter(t, api.target(t))
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	families := gather(t, reg)
	b := bLabels("m-1", "test-host", "photos")
	expectValue(t, families, "duplicati_backup_failure", b, 1)
	expectValue(t, families, "duplicati_machine_notifications_count",
		map[string]string{"machine_id": "m-1", "machine_name": "test-host", "backup_name": "photos", "type": "Error"}, 1)
	expectValue(t, families, "duplicati_machine_notifications_count",
		map[string]string{"machine_id": "m-1", "machine_name": "test-host", "backup_name": "photos", "type": "Warning"}, 1)
}

func TestCollectFailureClearsWithoutErrorNotification(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}
	api.notifications = []duplicati.Notification{
		{ID: 1, Type: "Information", BackupID: "b1"},
	}

	exp, _, reg := newTestExporter(t, api.target(t))
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	expectValue(t, gather(t, reg), "duplicati_backup_failure", bLabels("m-1", "test-host", "photos"), 0)
}

func TestPollAllMachinesDown(t *testing.T) {
	api := newFakeAPI(t)
	exp, _, _ := newTestExporter(t, api.target(t))
	api.fail.Store(true)

	if err := exp.Poll(context.Background()); !errors.Is(err, ErrAllMachinesDown) {
		t.Fatalf("Poll() error = %v, want ErrAllMachinesDown", err)
	}
}

func TestPollPartialFailureKeepsOtherMachines(t *testing.T) {
	up := newFakeAPI(t)
	down := newFakeAPI(t)
	down.machineName = "down-host"
	down.machineID = "m-2"
	now := time.Now().UTC()
	payload := healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))
	up.backups = []duplicati.BackupWithSchedule{payload}
	down.backups = []duplicati.BackupWithSchedule{payload}

	exp, _, reg := newTestExporter(t, up.target(t), down.target(t))
	down.fail.Store(true)

	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v, want nil for partial failure", err)
	}

	families := gather(t, reg)
	expectValue(t, families, "duplicati_machine_up", mLabels("m-1", "test-host"), 1)
	expectValue(t, families, "duplicati_machine_scrape_error", mLabels("m-1", "test-host"), 0)
	expectValue(t, families, "duplicati_machine_up", mLabels("m-2", "down-host"), 0)
	expectValue(t, families, "duplicati_machine_scrape_error", mLabels("m-2", "down-host"), 1)
}

func TestPollFailedMachineKeepsLastKnownBackupData(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}

	exp, _, reg := newTestExporter(t, api.target(t))
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("first Poll() error = %v", err)
	}
	expectValue(t, gather(t, reg), "duplicati_backup_list_count", bLabels("m-1", "test-host", "photos"), 7)

	api.fail.Store(true)
	if err := exp.Poll(context.Background()); !errors.Is(err, ErrAllMachinesDown) {
		t.Fatalf("second Poll() error = %v, want ErrAllMachinesDown", err)
	}
	expectValue(t, gather(t, reg), "duplicati_backup_list_count", bLabels("m-1", "test-host", "photos"), 7)
	expectValue(t, gather(t, reg), "duplicati_machine_scrape_error", mLabels("m-1", "test-host"), 1)
}

func TestCollectWebhookMetrics(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}

	target := api.target(t)
	exp, store, reg := newTestExporter(t, target)
	store.Put(&webhook.Report{
		MachineID:      "m-1",
		BackupName:     "photos",
		ParsedResult:   "Warning",
		BytesUploaded:  1024,
		FilesUploaded:  3,
		FilesDeleted:   1,
		FoldersCreated: 2,
		RetryAttempts:  4,
		Duration:       90 * time.Second,
		BeginTime:      now.Add(-time.Hour),
		EndTime:        now.Add(-time.Hour).Add(90 * time.Second),
	})

	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	families := gather(t, reg)
	b := bLabels("m-1", "test-host", "photos")
	expectValue(t, families, "duplicati_backup_bytes_uploaded_total", b, 1024)
	expectValue(t, families, "duplicati_backup_files_uploaded_total", b, 3)
	expectValue(t, families, "duplicati_backup_files_deleted_total", b, 1)
	expectValue(t, families, "duplicati_backup_folders_created_total", b, 2)
	expectValue(t, families, "duplicati_backup_retry_attempts", b, 4)
	expectValue(t, families, "duplicati_backup_last_result", b, 1)
	expectValue(t, families, "duplicati_backup_last_run_duration_seconds", b, 90)
	expectValue(t, families, "duplicati_backup_last_run_end_time", b,
		float64(now.Add(-time.Hour).Add(90*time.Second).Unix()))
}

func TestCollectFilesets(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}
	api.filesets["b1"] = []duplicati.Fileset{
		{Version: 1, IsFullBackup: 1, Time: now.Add(-48 * time.Hour).Format(time.RFC3339), FileCount: 10, FileSizes: 500},
		{Version: 2, IsFullBackup: 0, Time: now.Add(-24 * time.Hour).Format(time.RFC3339), FileCount: 12, FileSizes: 600},
	}

	exp, _, reg := newTestExporter(t, api.target(t))
	exp.opts.FilesetsEnabled = true

	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	families := gather(t, reg)
	expectValue(t, families, "duplicati_backup_filesets_count", bLabels("m-1", "test-host", "photos"), 2)
	expectValue(t, families, "duplicati_backup_fileset_is_full",
		map[string]string{"machine_id": "m-1", "machine_name": "test-host", "backup_name": "photos", "version": "1"}, 1)
	expectValue(t, families, "duplicati_backup_fileset_file_count",
		map[string]string{"machine_id": "m-1", "machine_name": "test-host", "backup_name": "photos", "version": "2"}, 12)

	// A second poll must reuse the cache and not hit the API again.
	calls := atomic.LoadInt32(&api.filesetCalls)
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("second Poll() error = %v", err)
	}
	if got := atomic.LoadInt32(&api.filesetCalls); got != calls {
		t.Errorf("fileset API calls = %d, want %d (cache should be used)", got, calls)
	}
}

func TestFilesetCacheTTLZeroDisablesCaching(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}

	exp, _, _ := newTestExporter(t, api.target(t))
	exp.opts.FilesetsEnabled = true
	exp.opts.FilesetsCacheTTL = 0

	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	calls := atomic.LoadInt32(&api.filesetCalls)
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("second Poll() error = %v", err)
	}
	if got := atomic.LoadInt32(&api.filesetCalls); got <= calls {
		t.Errorf("fileset API calls = %d, want > %d (TTL 0 must not cache)", got, calls)
	}
}

func TestFilesetFailuresAreCached(t *testing.T) {
	exp, _, _ := newTestExporter(t)
	exp.storeFilesets("k", nil, false)

	if _, hit, ok := exp.cachedFilesets("k"); !hit || ok {
		t.Errorf("cachedFilesets() hit=%v ok=%v, want hit=true ok=false", hit, ok)
	}
	exp.filesets["k"] = filesetCacheEntry{at: time.Now().Add(-2 * filesetFailureTTL), err: errors.New("x")}
	if _, hit, _ := exp.cachedFilesets("k"); hit {
		t.Error("expired failure entry must not be a hit")
	}
}

func TestPollIgnoresCallerCancelation(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}
	exp, _, reg := newTestExporter(t, api.target(t))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := exp.Poll(ctx); err != nil {
		t.Fatalf("Poll() with canceled context error = %v", err)
	}
	expectValue(t, gather(t, reg), "duplicati_machine_up", mLabels("m-1", "test-host"), 1)
}

func TestPollConcurrentCallersShareOnePoll(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}
	exp, _, _ := newTestExporter(t, api.target(t))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := exp.Poll(context.Background()); err != nil {
				t.Errorf("Poll() error = %v", err)
			}
		}()
	}
	wg.Wait()
	if got := exp.pollSeq.Load(); got >= 8 {
		t.Errorf("polls executed = %d, want fewer than the 8 callers", got)
	}
}

func TestIdentityRefreshOnlyWhenDue(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}
	exp, _, reg := newTestExporter(t, api.target(t))

	api.machineName = "renamed"
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	expectValue(t, gather(t, reg), "duplicati_machine_up", mLabels("m-1", "test-host"), 1)

	exp.opts.IdentityRefreshInterval = time.Nanosecond
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	expectValue(t, gather(t, reg), "duplicati_machine_up", mLabels("m-1", "renamed"), 1)
}

func TestCollectPausedMachine(t *testing.T) {
	api := newFakeAPI(t)
	api.programState = "Paused"
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}

	exp, _, reg := newTestExporter(t, api.target(t))
	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	expectValue(t, gather(t, reg), "duplicati_machine_paused", mLabels("m-1", "test-host"), 1)
}

func TestCollectNotificationFailureDoesNotFailScrape(t *testing.T) {
	api := newFakeAPI(t)
	now := time.Now().UTC()
	api.backups = []duplicati.BackupWithSchedule{healthyBackup(now.Add(24*time.Hour), now.Add(-time.Hour), now.Add(-time.Hour))}

	exp, _, reg := newTestExporter(t, api.target(t))
	api.notifyFail.Store(true)

	if err := exp.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	families := gather(t, reg)
	expectValue(t, families, "duplicati_machine_up", mLabels("m-1", "test-host"), 1)
	if _, ok := metricValue(t, families, "duplicati_backup_failure", bLabels("m-1", "test-host", "photos")); ok {
		t.Error("failure metric emitted although notifications could not be read")
	}
}
