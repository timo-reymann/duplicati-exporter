// Package webhook stores the most recent Duplicati send-http report per backup.
//
// The webhook is a secondary data source: the Duplicati HTTP API is the source of
// truth for durability, the report only contributes per-run transfer deltas and
// the parsed result of the last run.
package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/timo-reymann/duplicati-exporter/internal/duplicati"
)

// maxBodyBytes caps the accepted webhook payload.
const maxBodyBytes = 8 << 20 // 8 MiB

// ErrUnsupported is returned for payloads the exporter cannot interpret.
var ErrUnsupported = errors.New("webhook: unsupported payload")

// Report is the normalized content of one Duplicati JSON report.
type Report struct {
	MachineID      string
	MachineName    string
	BackupID       string
	BackupName     string
	Operation      string
	ParsedResult   string
	Interrupted    bool
	FilesWithError int64

	BeginTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
	ReceivedAt time.Time

	BytesUploaded   int64
	BytesDownloaded int64
	FilesUploaded   int64
	FilesDownloaded int64
	FilesDeleted    int64
	FoldersCreated  int64
	RetryAttempts   int64
}

// wireFormat mirrors Duplicati's JsonFormatSerializer output.
type wireFormat struct {
	Data      json.RawMessage   `json:"Data"`
	Extra     map[string]string `json:"Extra"`
	LogLines  []string          `json:"LogLines"`
	Exception *string           `json:"Exception"`
}

type wireData struct {
	MainOperation     string                 `json:"MainOperation"`
	ParsedResult      string                 `json:"ParsedResult"`
	Interrupted       bool                   `json:"Interrupted"`
	FilesWithError    int64                  `json:"FilesWithError"`
	BeginTime         string                 `json:"BeginTime"`
	EndTime           string                 `json:"EndTime"`
	Duration          string                 `json:"Duration"`
	BackendStatistics *wireBackendStatistics `json:"BackendStatistics"`
}

type wireBackendStatistics struct {
	RemoteCalls     int64 `json:"RemoteCalls"`
	BytesUploaded   int64 `json:"BytesUploaded"`
	BytesDownloaded int64 `json:"BytesDownloaded"`
	FilesUploaded   int64 `json:"FilesUploaded"`
	FilesDownloaded int64 `json:"FilesDownloaded"`
	FilesDeleted    int64 `json:"FilesDeleted"`
	FoldersCreated  int64 `json:"FoldersCreated"`
	RetryAttempts   int64 `json:"RetryAttempts"`
}

// Parse decodes a Duplicati JSON report body.
func Parse(body []byte) (*Report, error) {
	var wire wireFormat
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	if len(wire.Data) == 0 {
		return nil, fmt.Errorf("%w: missing Data section", ErrUnsupported)
	}

	var data wireData
	if err := json.Unmarshal(wire.Data, &data); err != nil {
		return nil, fmt.Errorf("%w: invalid Data section: %v", ErrUnsupported, err)
	}

	extra := map[string]string{}
	for k, v := range wire.Extra {
		extra[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	r := &Report{
		MachineID:      extra["machine-id"],
		MachineName:    extra["machine-name"],
		BackupID:       extra["backup-id"],
		BackupName:     extra["backup-name"],
		Operation:      firstNonEmpty(data.MainOperation, extra["operationname"]),
		ParsedResult:   data.ParsedResult,
		Interrupted:    data.Interrupted,
		FilesWithError: data.FilesWithError,
		Duration:       parseDuration(data.Duration),
		ReceivedAt:     time.Now().UTC(),
	}
	if r.ParsedResult == "" {
		r.ParsedResult = extra["parsedresult"]
	}
	if r.Operation == "" {
		r.Operation = extra["operationname"]
	}

	r.BeginTime = parseTime(data.BeginTime)
	r.EndTime = parseTime(data.EndTime)

	if s := data.BackendStatistics; s != nil {
		r.BytesUploaded = s.BytesUploaded
		r.BytesDownloaded = s.BytesDownloaded
		r.FilesUploaded = s.FilesUploaded
		r.FilesDownloaded = s.FilesDownloaded
		r.FilesDeleted = s.FilesDeleted
		r.FoldersCreated = s.FoldersCreated
		r.RetryAttempts = s.RetryAttempts
	}

	if r.BackupName == "" && r.BackupID == "" {
		return nil, fmt.Errorf("%w: report carries neither backup-name nor backup-id", ErrUnsupported)
	}
	return r, nil
}

// ParseBody interprets an HTTP request body according to its content type.
// Duplicati posts either application/json (send-http-result-output-format=Json)
// or application/x-www-form-urlencoded with a message= parameter.
func ParseBody(contentType string, body []byte) (*Report, error) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))

	switch mediaType {
	case "", "application/json", "text/json", "text/plain":
		return Parse(body)
	case "application/x-www-form-urlencoded":
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, fmt.Errorf("%w: malformed form body", ErrUnsupported)
		}
		msg := form.Get("message")
		if msg == "" {
			return nil, fmt.Errorf("%w: form body has no message field", ErrUnsupported)
		}
		return Parse([]byte(msg))
	default:
		return nil, fmt.Errorf("%w: content type %q", ErrUnsupported, contentType)
	}
}

// Decode drains and decodes an io.Reader limited to maxBodyBytes.
func Decode(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// Key identifies a stored report.
type Key struct {
	MachineID  string
	BackupName string
}

// Store keeps the latest report per machine and backup. It is safe for
// concurrent use.
type Store struct {
	mu sync.RWMutex
	m  map[Key]*Report
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{m: make(map[Key]*Report)}
}

// Put records a report, overwriting any previous report for the same key.
func (s *Store) Put(r *Report) {
	if r == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[Key]*Report)
	}
	key := Key{MachineID: r.MachineID, BackupName: r.BackupName}
	if r.MachineID == "" {
		key.MachineID = r.MachineName
	}
	s.m[key] = r
}

// Lookup finds the report for a backup of a specific machine. It first matches
// the machine id, then the machine name, so reports work whether or not the
// machine id was discovered from the backup settings.
func (s *Store) Lookup(machineID, machineName, backupName string) (*Report, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, id := range []string{machineID, machineName} {
		if id == "" {
			continue
		}
		if r, ok := s.m[Key{MachineID: id, BackupName: backupName}]; ok {
			return r, true
		}
	}
	// Fall back to a unique match on the backup name alone.
	var found *Report
	for k, r := range s.m {
		if k.BackupName != backupName {
			continue
		}
		if found != nil && found != r {
			return nil, false
		}
		found = r
	}
	return found, found != nil
}

// Len returns the number of stored reports.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func parseDuration(raw string) time.Duration {
	if d, err := duplicati.ParseDuration(raw); err == nil {
		return d
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
