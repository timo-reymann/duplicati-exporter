package duplicati

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrUnauthorized is returned when Duplicati rejects the credentials.
var ErrUnauthorized = errors.New("duplicati: unauthorized")

// ErrNotFound is returned for HTTP 404 responses.
var ErrNotFound = errors.New("duplicati: not found")

// maxResponseBytes caps how much of a response is read. The systeminfo payload
// carries the full option catalog and is by far the largest response.
const maxResponseBytes = 32 << 20 // 32 MiB

// Client talks to one Duplicati server. It is safe for concurrent use.
type Client struct {
	baseURL *url.URL
	client  *http.Client

	password string
	fixedTok string

	mu    sync.Mutex
	token string
}

// NewClient builds a client for the given endpoint.
func NewClient(baseURL *url.URL, password, token string, insecureSkipVerify bool, timeout time.Duration) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in via --server flag
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL:  baseURL,
		client:   &http.Client{Timeout: timeout, Transport: transport},
		password: password,
		fixedTok: token,
	}
}

// BaseURL returns the configured endpoint.
func (c *Client) BaseURL() *url.URL { return c.baseURL }

func (c *Client) endpoint(path string) string {
	base := strings.TrimSuffix(c.baseURL.String(), "/")
	return base + path
}

// accessToken returns a bearer token, logging in when a password is configured.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	if c.fixedTok != "" {
		return c.fixedTok, nil
	}

	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok != "" {
		return tok, nil
	}
	return c.login(ctx)
}

// login exchanges the configured password for a short-lived access token.
func (c *Client) login(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]any{
		"Password":   c.password,
		"RememberMe": true,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/api/v1/auth/login"), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("duplicati: login to %s: %w", c.baseURL.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%w: login to %s rejected", ErrUnauthorized, c.baseURL.Host)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", readAPIError(resp)
	}

	var out struct {
		AccessToken string `json:"AccessToken"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("duplicati: decode login response from %s: %w", c.baseURL.Host, err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("duplicati: login to %s returned no access token", c.baseURL.Host)
	}

	c.mu.Lock()
	c.token = out.AccessToken
	c.mu.Unlock()
	return out.AccessToken, nil
}

// invalidate drops the cached token so the next call logs in again.
func (c *Client) invalidate() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
}

// getJSON performs an authenticated GET and decodes the response into out.
// A 404 is reported as ErrNotFound. The request is retried once after a 401 by
// refreshing the token.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body io.Reader, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := c.accessToken(ctx)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("duplicati: %s %s: %w", method, path, err)
		}

		if resp.StatusCode == http.StatusUnauthorized {
			_ = resp.Body.Close()
			if attempt == 0 && c.fixedTok == "" {
				c.invalidate()
				continue
			}
			return fmt.Errorf("%w: %s %s", ErrUnauthorized, method, path)
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return fmt.Errorf("%w: %s %s", ErrNotFound, method, path)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer func() { _ = resp.Body.Close() }()
			return readAPIError(resp)
		}

		err = json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("duplicati: decode %s %s: %w", method, path, err)
		}
		return nil
	}
	return fmt.Errorf("duplicati: %s %s: exhausted retries", method, path)
}

// SystemInfo fetches server and machine identity information.
func (c *Client) SystemInfo(ctx context.Context) (*SystemInfo, error) {
	var out SystemInfo
	if err := c.getJSON(ctx, "/api/v1/systeminfo", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ServerSettings fetches the server-wide settings. Values are normalised to
// strings because Duplicati serialises booleans and numbers inconsistently.
func (c *Client) ServerSettings(ctx context.Context) (map[string]string, error) {
	var raw map[string]any
	if err := c.getJSON(ctx, "/api/v1/serversettings", &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out, nil
}

// ServerState fetches the scheduler state.
func (c *Client) ServerState(ctx context.Context) (*ServerState, error) {
	var out ServerState
	if err := c.getJSON(ctx, "/api/v1/serverstate", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Backups lists all backup configurations together with their schedule.
func (c *Client) Backups(ctx context.Context) ([]BackupWithSchedule, error) {
	var out []BackupWithSchedule
	if err := c.getJSON(ctx, "/api/v1/backups", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Backup fetches a single backup configuration.
func (c *Client) Backup(ctx context.Context, id string) (*BackupDetail, error) {
	var out BackupDetail
	if err := c.getJSON(ctx, "/api/v1/backup/"+url.PathEscape(id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Filesets lists the stored versions of a backup. Listing versions runs a
// Duplicati list operation against the backend, so failures must be tolerated.
func (c *Client) Filesets(ctx context.Context, id string) ([]Fileset, error) {
	var out []Fileset
	if err := c.getJSON(ctx, "/api/v1/backup/"+url.PathEscape(id)+"/filesets", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Notifications lists the pending notifications of the server.
func (c *Client) Notifications(ctx context.Context) ([]Notification, error) {
	var out []Notification
	if err := c.getJSON(ctx, "/api/v1/notifications", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ProgressState returns the state of the currently running task. ErrNotFound is
// returned when no task is running.
func (c *Client) ProgressState(ctx context.Context) (*ProgressState, error) {
	var out ProgressState
	if err := c.getJSON(ctx, "/api/v1/progressstate", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func readAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	msg := strings.TrimSpace(string(body))
	// Duplicati may wrap the message in {"Message":"..."}.
	var wrapped struct {
		Message string `json:"Message"`
	}
	if json.Unmarshal(body, &wrapped) == nil && wrapped.Message != "" {
		msg = wrapped.Message
	}
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("duplicati: HTTP %d: %s", resp.StatusCode, msg)
}
