package webhook

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const sampleReport = `{
  "Data": {
    "MainOperation": "Backup",
    "ParsedResult": "Success",
    "Interrupted": false,
    "FilesWithError": 2,
    "BeginTime": "2026-10-06T10:00:00Z",
    "EndTime": "2026-10-06T10:05:00Z",
    "Duration": "00:05:00",
    "BackendStatistics": {
      "RemoteCalls": 3,
      "BytesUploaded": 1048576,
      "BytesDownloaded": 4096,
      "FilesUploaded": 12,
      "FilesDownloaded": 1,
      "FilesDeleted": 2,
      "FoldersCreated": 1,
      "RetryAttempts": 0
    }
  },
  "Extra": {
    "machine-id": "m-1",
    "machine-name": "host-a",
    "backup-id": "b-1",
    "backup-name": "photos",
    "OperationName": "Backup"
  },
  "LogLines": ["line 1"],
  "Exception": null
}`

func TestParseJSONReport(t *testing.T) {
	r, err := Parse([]byte(sampleReport))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"machine id", r.MachineID, "m-1"},
		{"machine name", r.MachineName, "host-a"},
		{"backup id", r.BackupID, "b-1"},
		{"backup name", r.BackupName, "photos"},
		{"operation", r.Operation, "Backup"},
		{"parsed result", r.ParsedResult, "Success"},
		{"bytes uploaded", r.BytesUploaded, int64(1048576)},
		{"files uploaded", r.FilesUploaded, int64(12)},
		{"files deleted", r.FilesDeleted, int64(2)},
		{"folders created", r.FoldersCreated, int64(1)},
		{"retry attempts", r.RetryAttempts, int64(0)},
		{"files with error", r.FilesWithError, int64(2)},
		{"interrupted", r.Interrupted, false},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	if want := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC); !r.BeginTime.Equal(want) {
		t.Errorf("BeginTime = %v, want %v", r.BeginTime, want)
	}
	if want := 5 * time.Minute; r.Duration != want {
		t.Errorf("Duration = %v, want %v", r.Duration, want)
	}
	if r.ReceivedAt.IsZero() {
		t.Error("ReceivedAt is zero")
	}
}

func TestParseFormEncodedReport(t *testing.T) {
	body := "message=" + urlEncode(sampleReport)
	r, err := ParseBody("application/x-www-form-urlencoded", []byte(body))
	if err != nil {
		t.Fatalf("ParseBody() error = %v", err)
	}
	if r.BackupName != "photos" {
		t.Errorf("BackupName = %q, want photos", r.BackupName)
	}
}

func TestParseBodyJSON(t *testing.T) {
	r, err := ParseBody("application/json; charset=utf-8", []byte(sampleReport))
	if err != nil {
		t.Fatalf("ParseBody() error = %v", err)
	}
	if r.ParsedResult != "Success" {
		t.Errorf("ParsedResult = %q, want Success", r.ParsedResult)
	}
}

func TestParseBodyRejectsUnknownContentType(t *testing.T) {
	_, err := ParseBody("application/xml", []byte("<x/>"))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ParseBody() error = %v, want ErrUnsupported", err)
	}
}

func TestParseBodyRejectsDuplicatiTextFormat(t *testing.T) {
	body := "message=" + urlEncode("DeletedFiles: 0 ModifiedFiles: 1")
	_, err := ParseBody("application/x-www-form-urlencoded", []byte(body))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ParseBody() error = %v, want ErrUnsupported", err)
	}
}

func TestParseRejectsReportWithoutBackupIdentity(t *testing.T) {
	body := `{"Data":{"MainOperation":"Backup"},"Extra":{}}`
	_, err := Parse([]byte(body))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Parse() error = %v, want ErrUnsupported", err)
	}
}

func TestParseFallbackResultKeyIsCaseInsensitive(t *testing.T) {
	body := `{"Data":{"MainOperation":"Backup"},"Extra":{"backup-name":"x","parsedresult":"Warning"}}`
	r, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if r.ParsedResult != "Warning" {
		t.Errorf("ParsedResult = %q, want Warning", r.ParsedResult)
	}
}

func TestStoreLookupByMachineID(t *testing.T) {
	s := NewStore()
	s.Put(&Report{MachineID: "m-1", BackupName: "photos"})
	s.Put(&Report{MachineID: "m-2", BackupName: "photos"})

	r, ok := s.Lookup("m-1", "host-a", "photos")
	if !ok || r.MachineID != "m-1" {
		t.Fatalf("Lookup(m-1) = %+v, ok=%v", r, ok)
	}
	r, ok = s.Lookup("m-2", "host-b", "photos")
	if !ok || r.MachineID != "m-2" {
		t.Fatalf("Lookup(m-2) = %+v, ok=%v", r, ok)
	}
}

func TestStoreLookupFallsBackToMachineName(t *testing.T) {
	s := NewStore()
	s.Put(&Report{MachineName: "host-a", BackupName: "photos"})

	if _, ok := s.Lookup("unknown-id", "host-a", "photos"); !ok {
		t.Fatal("Lookup() by machine name failed")
	}
}

func TestStoreLookupAmbiguousBackupName(t *testing.T) {
	s := NewStore()
	s.Put(&Report{MachineID: "m-1", BackupName: "photos"})
	s.Put(&Report{MachineID: "m-2", BackupName: "photos"})

	if _, ok := s.Lookup("m-3", "host-c", "photos"); ok {
		t.Error("Lookup() returned a match for an ambiguous backup name")
	}
}

func TestStoreLookupMiss(t *testing.T) {
	s := NewStore()
	if _, ok := s.Lookup("m-1", "host-a", "missing"); ok {
		t.Error("Lookup() matched an empty store")
	}
	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0", s.Len())
	}
}

func TestStoreOverwritesExistingReport(t *testing.T) {
	s := NewStore()
	s.Put(&Report{MachineID: "m-1", BackupName: "photos", FilesUploaded: 1})
	s.Put(&Report{MachineID: "m-1", BackupName: "photos", FilesUploaded: 9})

	r, ok := s.Lookup("m-1", "host-a", "photos")
	if !ok {
		t.Fatal("Lookup() failed")
	}
	if r.FilesUploaded != 9 {
		t.Errorf("FilesUploaded = %d, want 9", r.FilesUploaded)
	}
	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}
}

func TestDecodeLimitsBody(t *testing.T) {
	body, err := Decode(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if string(body) != "hello" {
		t.Errorf("Decode() = %q, want hello", body)
	}
}

func urlEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			const hex = "0123456789ABCDEF"
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return b.String()
}
