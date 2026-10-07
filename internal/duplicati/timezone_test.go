package duplicati

import (
	"testing"
	"time"
)

func TestParseTimestamp(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}

	tests := []struct {
		name string
		raw  string
		loc  *time.Location
		want time.Time
		ok   bool
	}{
		{name: "rfc3339 utc", raw: "2026-10-06T00:00:00Z", loc: time.UTC, want: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "rfc3339 offset", raw: "2026-10-06T02:00:00+02:00", loc: time.UTC, want: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "duplicati utc", raw: "20261006T000000Z", loc: time.UTC, want: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "unspecified uses server tz", raw: "2026-10-06T02:00:00", loc: berlin, want: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "duplicati no zone uses server tz", raw: "20261006T020000", loc: berlin, want: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "date only", raw: "2026-10-06", loc: time.UTC, want: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "empty", raw: "", loc: time.UTC, want: time.Time{}, ok: true},
		{name: "dotnet zero", raw: "0001-01-01T00:00:00", loc: time.UTC, want: time.Time{}, ok: true},
		{name: "garbage", raw: "not-a-time", loc: time.UTC, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimestamp(tt.raw, tt.loc)
			if tt.ok {
				if err != nil {
					t.Fatalf("ParseTimestamp(%q) error = %v", tt.raw, err)
				}
				if !got.Equal(tt.want) {
					t.Errorf("ParseTimestamp(%q) = %v, want %v", tt.raw, got.UTC(), tt.want.UTC())
				}
				if got.Location() != time.UTC {
					t.Errorf("ParseTimestamp(%q) location = %v, want UTC", tt.raw, got.Location())
				}
			} else if err == nil {
				t.Errorf("ParseTimestamp(%q) error = nil, want error", tt.raw)
			}
		})
	}
}

func TestParseTimestampUnix(t *testing.T) {
	got := ParseTimestampUnix("2026-10-06T00:00:00Z", time.UTC)
	want := float64(time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC).Unix())
	if got != want {
		t.Errorf("ParseTimestampUnix() = %v, want %v", got, want)
	}
	if v := ParseTimestampUnix("nope", time.UTC); v != 0 {
		t.Errorf("ParseTimestampUnix(nope) = %v, want 0", v)
	}
	if v := ParseTimestampUnix("", time.UTC); v != 0 {
		t.Errorf("ParseTimestampUnix(empty) = %v, want 0", v)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		{"", 0, true},
		{"00:00:00", 0, true},
		{"00:01:30", 90 * time.Second, true},
		{"1.00:00:00", 24 * time.Hour, true},
		{"1.02:03:04", 24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second, true},
		{"00:00:01.500", 1500 * time.Millisecond, true},
		{"-00:01:00", -time.Minute, true},
		{"PT1H30M", 90 * time.Minute, true},
		{"P1DT2H", 26 * time.Hour, true},
		{"45", 45 * time.Second, true},
		{"nonsense", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseDuration(tt.raw)
			if tt.ok {
				if err != nil {
					t.Fatalf("ParseDuration(%q) error = %v", tt.raw, err)
				}
				if got != tt.want {
					t.Errorf("ParseDuration(%q) = %v, want %v", tt.raw, got, tt.want)
				}
			} else if err == nil {
				t.Errorf("ParseDuration(%q) error = nil, want error", tt.raw)
			}
		})
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		raw  string
		want float64
	}{
		{"", 0},
		{"1024", 1024},
		{"1,024", 1024},
		{"10KB", 10},
		{"-5", -5},
		{"not-a-number", 0},
	}
	for _, tt := range tests {
		if got := ParseInt64(tt.raw); got != tt.want {
			t.Errorf("ParseInt64(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestResolveLocation(t *testing.T) {
	if _, err := time.LoadLocation("Europe/Berlin"); err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}

	t.Run("iana id", func(t *testing.T) {
		loc, err := ResolveLocation("Europe/Berlin", "")
		if err != nil {
			t.Fatalf("ResolveLocation() error = %v", err)
		}
		if loc.String() != "Europe/Berlin" {
			t.Errorf("location = %q, want Europe/Berlin", loc.String())
		}
	})

	t.Run("utc", func(t *testing.T) {
		loc, err := ResolveLocation("UTC", "")
		if err != nil {
			t.Fatalf("ResolveLocation() error = %v", err)
		}
		if loc != time.UTC {
			t.Errorf("location = %v, want UTC", loc)
		}
	})

	t.Run("windows id falls back to offset in server time", func(t *testing.T) {
		loc, err := ResolveLocation("W. Europe Standard Time", "2026-10-06T14:30:00+02:00")
		if err != nil {
			t.Fatalf("ResolveLocation() error = %v", err)
		}
		if _, off := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC).In(loc).Zone(); off != 2*3600 {
			t.Errorf("offset = %d, want 7200", off)
		}
	})

	t.Run("unknown timezone without offset fails", func(t *testing.T) {
		if _, err := ResolveLocation("Not/AZone", ""); err == nil {
			t.Error("ResolveLocation() error = nil, want error")
		}
	})
}
