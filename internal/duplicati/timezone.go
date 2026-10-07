package duplicati

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// duplicatiTimeLayout is the layout Duplicati's Utility.SerializeDateTime emits,
// e.g. "20261006T000000Z". It always denotes UTC.
const duplicatiTimeLayout = "20060102T150405Z"

// duplicatiTimeLayoutNoZone is the same layout without the trailing "Z" and is
// interpreted in the server's timezone.
const duplicatiTimeLayoutNoZone = "20060102T150405"

// ParseTimestamp converts any of Duplicati's timestamp representations into a
// UTC instant. Timestamps that carry no zone information are interpreted in loc,
// which must be the server's timezone discovered at startup.
func ParseTimestamp(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0001-01-01T00:00:00" || raw == "0001-01-01T00:00:00Z" {
		return time.Time{}, nil
	}
	if loc == nil {
		loc = time.UTC
	}

	// Zone aware formats first.
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(duplicatiTimeLayout, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC1123, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC1123Z, raw); err == nil {
		return t.UTC(), nil
	}

	// No zone information: interpret in the server timezone.
	for _, layout := range []string{
		duplicatiTimeLayoutNoZone,
		time.RFC3339, // no offset case handled above via Parse
		"2006-01-02T15:04:05.9999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			// time.Parse yields UTC for layouts without a zone; rebuild the wall
			// clock in the server timezone so the instant is correct.
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc).UTC(), nil
		}
	}

	// .NET serializes Unspecified-kind DateTime without offset, sometimes with
	// a trailing lowercase "z" which RFC3339 does not accept.
	trimmed := strings.TrimSuffix(raw, "z")
	if trimmed != raw {
		if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized timestamp %q", raw)
}

// ParseTimestampUnix is ParseTimestamp returning Unix seconds. It returns 0 when
// the value is empty or unparsable, which renders as "no data" for time gauges.
func ParseTimestampUnix(raw string, loc *time.Location) float64 {
	t, err := ParseTimestamp(raw, loc)
	if err != nil || t.IsZero() {
		return 0
	}
	return float64(t.Unix())
}

var netDurationRe = regexp.MustCompile(`^(-)?(?:(\d+)\.)?(\d{1,2}):(\d{2}):(\d{2})(?:\.(\d+))?$`)
var isoDurationRe = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:([\d.]+)S)?)?$`)

// ParseDuration understands the duration formats that show up in the Duplicati
// API: .NET TimeSpan ("1.02:03:04", "00:03:04") and ISO 8601 ("PT1H30M").
func ParseDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "00:00:00" {
		return 0, nil
	}

	if m := netDurationRe.FindStringSubmatch(raw); m != nil {
		sign := 1.0
		if m[1] == "-" {
			sign = -1
		}
		var days, hours, mins, secs float64
		if m[2] != "" {
			days = toFloat(m[2])
		}
		hours = toFloat(m[3])
		mins = toFloat(m[4])
		secs = toFloat(m[5])
		if m[6] != "" {
			frac := "0." + m[6]
			secs += toFloat(frac)
		}
		total := sign * ((days*24+hours)*float64(time.Hour) + mins*float64(time.Minute) + secs*float64(time.Second))
		return time.Duration(total), nil
	}

	if m := isoDurationRe.FindStringSubmatch(raw); m != nil && strings.HasPrefix(raw, "P") {
		var total float64
		if m[1] != "" {
			total += toFloat(m[1]) * 24 * float64(time.Hour)
		}
		if m[2] != "" {
			total += toFloat(m[2]) * float64(time.Hour)
		}
		if m[3] != "" {
			total += toFloat(m[3]) * float64(time.Minute)
		}
		if m[4] != "" {
			total += toFloat(m[4]) * float64(time.Second)
		}
		return time.Duration(total), nil
	}

	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Duration(n * float64(time.Second)), nil
	}

	return 0, fmt.Errorf("unrecognized duration %q", raw)
}

// ParseDurationSeconds is ParseDuration returning a float number of seconds.
// It returns 0 when the value is empty or unparsable.
func ParseDurationSeconds(raw string) float64 {
	d, err := ParseDuration(raw)
	if err != nil {
		return 0
	}
	return d.Seconds()
}

// ParseInt64 parses a metadata value as an integer, tolerating thousands
// separators and empty values.
func ParseInt64(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	// Duplicati renders some sizes with a unit suffix; keep only the number.
	if i := strings.IndexAny(raw, "KMGTkmgt"); i > 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	raw = strings.ReplaceAll(raw, ",", "")
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return n
}

func toFloat(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}
