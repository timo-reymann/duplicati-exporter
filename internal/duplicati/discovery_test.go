package duplicati

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDiscoverSuccess(t *testing.T) {
	f := newFakeDuplicati(t)
	f.backups = []BackupWithSchedule{{
		Backup: Backup{
			ID:            "1",
			Name:          "photos",
			OperationType: "Backup",
			Settings:      []Setting{{Name: "--machine-id", Value: "abc-123"}},
			Metadata:      map[string]string{"LastBackupFinished": "20261006T100000Z"},
		},
		Schedule: &Schedule{ID: 1, Time: "2026-10-07T02:00:00Z", Repeat: "1D"},
	}}

	info, err := Discover(context.Background(), f.client(t))
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	id, loc := info.Snapshot()
	if id.MachineID != "abc-123" {
		t.Errorf("MachineID = %q, want abc-123", id.MachineID)
	}
	if id.MachineName != "test-host" {
		t.Errorf("MachineName = %q, want test-host", id.MachineName)
	}
	if id.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", id.Timezone)
	}
	if loc == nil {
		t.Fatal("location is nil")
	}
	if info.DiscoveredAt().IsZero() {
		t.Error("DiscoveredAt() is zero")
	}
}

func TestDiscoverFallsBackToHostForMachineID(t *testing.T) {
	f := newFakeDuplicati(t)
	f.backups = []BackupWithSchedule{{
		Backup: Backup{ID: "1", Name: "photos", OperationType: "Backup"},
	}}

	info, err := Discover(context.Background(), f.client(t))
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	id, _ := info.Snapshot()
	want := f.server.Listener.Addr().String()
	if id.MachineID != want {
		t.Errorf("MachineID = %q, want %q (endpoint host fallback)", id.MachineID, want)
	}
}

func TestDiscoverPrefersServerSettings(t *testing.T) {
	f := newFakeDuplicati(t)
	f.serverSettings = map[string]string{
		"--machine-name": "Photoprism",
		"--machine-id":   "photoprism-pi",
		"is-first-run":   "",
	}
	f.backups = []BackupWithSchedule{{
		Backup: Backup{
			ID: "1", Name: "photos", OperationType: "Backup",
			Settings: []Setting{{Name: "--machine-id", Value: "from-backup"}},
		},
	}}

	info, err := Discover(context.Background(), f.client(t))
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	id, _ := info.Snapshot()
	if id.MachineName != "Photoprism" {
		t.Errorf("MachineName = %q, want Photoprism", id.MachineName)
	}
	if id.MachineID != "photoprism-pi" {
		t.Errorf("MachineID = %q, want photoprism-pi", id.MachineID)
	}

	// A refresh must keep honouring the server settings and pick up renames.
	f.serverSettings["--machine-name"] = "Photoprism 2"
	if err := info.Refresh(context.Background(), f.client(t)); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	id, _ = info.Snapshot()
	if id.MachineName != "Photoprism 2" {
		t.Errorf("MachineName after refresh = %q, want Photoprism 2", id.MachineName)
	}
	if id.MachineID != "photoprism-pi" {
		t.Errorf("MachineID after refresh = %q, want photoprism-pi", id.MachineID)
	}
}

func TestDiscoverIgnoresEmptyServerSettings(t *testing.T) {
	f := newFakeDuplicati(t)
	f.serverSettings = map[string]string{"--machine-name": "  ", "--machine-id": ""}

	info, err := Discover(context.Background(), f.client(t))
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	id, _ := info.Snapshot()
	if id.MachineName != "test-host" {
		t.Errorf("MachineName = %q, want systeminfo fallback test-host", id.MachineName)
	}
}

func TestDiscoverFailsWithoutAuth(t *testing.T) {
	f := newFakeDuplicati(t)
	f.failLogin = true

	if _, err := Discover(context.Background(), f.client(t)); err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
}

func TestDiscoverFailsOnUnreachableEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	c := NewClient(u, "secret", "", false, time.Second)
	if _, err := Discover(context.Background(), c); err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
}

func TestDiscoverFailsOnEmptyMachineName(t *testing.T) {
	f := newFakeDuplicati(t)
	f.systemInfo.MachineName = ""

	if _, err := Discover(context.Background(), f.client(t)); err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
}

func TestDiscoverFailsOnUnknownTimezone(t *testing.T) {
	f := newFakeDuplicati(t)
	f.systemInfo.ServerTimeZone = "Not/AZone"

	_, err := Discover(context.Background(), f.client(t))
	if err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
}

func TestRefreshUpdatesIdentity(t *testing.T) {
	f := newFakeDuplicati(t)
	info, err := Discover(context.Background(), f.client(t))
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	f.systemInfo.MachineName = "renamed-host"
	f.systemInfo.ServerVersionName = "2.1.1.0"
	if err := info.Refresh(context.Background(), f.client(t)); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	id, _ := info.Snapshot()
	if id.MachineName != "renamed-host" {
		t.Errorf("MachineName = %q, want renamed-host", id.MachineName)
	}
	if id.Version != "2.1.1.0" {
		t.Errorf("Version = %q, want 2.1.1.0", id.Version)
	}
	if id.MachineID == "" {
		t.Error("MachineID was cleared by Refresh()")
	}
}

func TestMachineIDFromBackupsIgnoresOtherOptions(t *testing.T) {
	backups := []BackupWithSchedule{
		{Backup: Backup{Settings: []Setting{{Name: "machine-name", Value: "host-a"}}}},
		{Backup: Backup{Settings: []Setting{{Name: "--machine-id", Value: "the-id"}}}},
	}
	if got := machineIDFromBackups(backups); got != "the-id" {
		t.Errorf("machineIDFromBackups() = %q, want the-id", got)
	}
}

func TestBackupOptionLookup(t *testing.T) {
	b := Backup{Settings: []Setting{{Name: "machine-id", Value: "  xyz  "}}}
	if got := b.Option("machine-id"); got != "xyz" {
		t.Errorf("Option() = %q, want xyz", got)
	}
	if got := b.Option("missing"); got != "" {
		t.Errorf("Option(missing) = %q, want empty", got)
	}
}
