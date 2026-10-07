package duplicati

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDuplicati is a minimal Duplicati API server for tests.
type fakeDuplicati struct {
	server        *httptest.Server
	password      string
	token         string
	failLogin     bool
	authChecks    int32
	failOn        map[string]bool
	systemInfo    *SystemInfo
	backups       []BackupWithSchedule
	serverState   *ServerState
	notifications []Notification
}

func newFakeDuplicati(t *testing.T) *fakeDuplicati {
	t.Helper()
	f := &fakeDuplicati{
		password: "secret",
		token:    "test-token",
		systemInfo: &SystemInfo{
			APIVersion:        1,
			ServerVersion:     "2.1.0.5",
			ServerVersionName: "2.1.0.5_beta",
			ServerTimeZone:    "UTC",
			MachineName:       "test-host",
			OSType:            "Linux",
			OSVersion:         "test",
		},
		serverState: &ServerState{ProgramState: "Running", HasWarning: false},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if f.failLogin {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct {
			Password string `json:"Password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Password != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"AccessToken": f.token, "RefreshNonce": nil})
	})

	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&f.authChecks, 1)
			if r.Header.Get("Authorization") != "Bearer "+f.token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}

	mux.HandleFunc("/api/v1/systeminfo", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.systemInfo)
	}))
	mux.HandleFunc("/api/v1/serverstate", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.serverState)
	}))
	mux.HandleFunc("/api/v1/backups", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.backups)
	}))
	mux.HandleFunc("/api/v1/notifications", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.notifications)
	}))
	mux.HandleFunc("/api/v1/progressstate", auth(func(w http.ResponseWriter, _ *http.Request) {
		if f.failOn["progressstate"] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"BackupID": "1", "TaskID": 42, "Phase": "Backup", "OverallProgress": 0.5,
		})
	}))

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeDuplicati) client(t *testing.T) *Client {
	t.Helper()
	u, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(u, f.password, "", false, 5*time.Second)
}

func TestClientLoginAndRequest(t *testing.T) {
	f := newFakeDuplicati(t)
	c := f.client(t)

	ctx := context.Background()
	info, err := c.SystemInfo(ctx)
	if err != nil {
		t.Fatalf("SystemInfo() error = %v", err)
	}
	if info.MachineName != "test-host" {
		t.Errorf("MachineName = %q, want test-host", info.MachineName)
	}
	if got := atomic.LoadInt32(&f.authChecks); got == 0 {
		t.Error("no authenticated request was made")
	}
}

func TestClientInvalidPassword(t *testing.T) {
	f := newFakeDuplicati(t)
	u, _ := url.Parse(f.server.URL)
	c := NewClient(u, "wrong", "", false, 5*time.Second)

	_, err := c.SystemInfo(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("SystemInfo() error = %v, want ErrUnauthorized", err)
	}
}

func TestClientReLoginOn401(t *testing.T) {
	f := newFakeDuplicati(t)
	c := f.client(t)
	ctx := context.Background()

	if _, err := c.SystemInfo(ctx); err != nil {
		t.Fatalf("SystemInfo() error = %v", err)
	}
	// Simulate a token that was revoked server side.
	f.token = "rotated"
	// The cached token is now invalid; the client must log in again.
	// Force the next request to see the old token by resetting ours.
	c.mu.Lock()
	c.token = "stale"
	c.mu.Unlock()

	// Server now expects the rotated token, so the stale one gets a 401 and the
	// client logs in again and picks up the new token.
	got, err := c.SystemInfo(ctx)
	if err != nil {
		t.Fatalf("SystemInfo() after rotation error = %v", err)
	}
	if got.MachineName != "test-host" {
		t.Errorf("MachineName = %q, want test-host", got.MachineName)
	}
}

func TestClientFixedToken(t *testing.T) {
	f := newFakeDuplicati(t)
	u, _ := url.Parse(f.server.URL)
	c := NewClient(u, "", f.token, false, 5*time.Second)

	if _, err := c.SystemInfo(context.Background()); err != nil {
		t.Fatalf("SystemInfo() error = %v", err)
	}
}

func TestClientProgressStateNotFound(t *testing.T) {
	f := newFakeDuplicati(t)
	f.failOn = map[string]bool{"progressstate": true}
	c := f.client(t)

	_, err := c.ProgressState(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ProgressState() error = %v, want ErrNotFound", err)
	}
}

func TestClientInsecureSkipVerify(t *testing.T) {
	f := newFakeDuplicati(t)
	u, _ := url.Parse(f.server.URL)
	c := NewClient(u, f.password, "", true, 5*time.Second)
	if _, err := c.SystemInfo(context.Background()); err != nil {
		t.Fatalf("SystemInfo() error = %v", err)
	}
}
