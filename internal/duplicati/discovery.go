package duplicati

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MachineIdentity is the immutable part of the discovered machine metadata.
type MachineIdentity struct {
	MachineID   string
	MachineName string
	Timezone    string
	Version     string
	OSType      string
	OSVersion   string
}

// MachineInfo holds the identity discovered for one Duplicati server. It is
// resolved once at startup and refreshed behind a mutex periodically.
type MachineInfo struct {
	mu sync.RWMutex
	MachineIdentity
	loc        *time.Location
	discovered time.Time
}

// Discover resolves the machine identity for one endpoint. It fails when the
// server is unreachable, authentication is rejected or the identity cannot be
// established, so that the exporter refuses to start in a degraded state.
func Discover(ctx context.Context, c *Client) (*MachineInfo, error) {
	sys, err := c.SystemInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover systeminfo: %w", err)
	}
	if strings.TrimSpace(sys.MachineName) == "" {
		return nil, fmt.Errorf("discover systeminfo: server reported an empty machine name")
	}

	backups, err := c.Backups(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover backups: %w", err)
	}

	loc, err := ResolveLocation(sys.ServerTimeZone, "")
	if err != nil {
		return nil, fmt.Errorf("discover timezone: %w", err)
	}

	id := MachineIdentity{
		MachineID:   machineIDFromBackups(backups),
		MachineName: strings.TrimSpace(sys.MachineName),
		Timezone:    sys.ServerTimeZone,
		Version:     firstNonEmpty(sys.ServerVersionName, sys.ServerVersion),
		OSType:      sys.OSType,
		OSVersion:   sys.OSVersion,
	}
	if id.MachineID == "" {
		// Duplicati only exposes machine-id as a backup option; when no backup
		// overrides it, fall back to a stable identifier derived from the
		// endpoint so the label set stays unique and stable.
		id.MachineID = c.BaseURL().Host
	}

	return &MachineInfo{MachineIdentity: id, loc: loc, discovered: time.Now().UTC()}, nil
}

// machineIDFromBackups returns the machine-id backup option if any backup
// declares one.
func machineIDFromBackups(backups []BackupWithSchedule) string {
	for _, b := range backups {
		if v := b.Backup.Option("machine-id"); v != "" {
			return v
		}
	}
	return ""
}

// Snapshot returns a consistent copy of the identity together with its location.
func (m *MachineInfo) Snapshot() (MachineIdentity, *time.Location) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.MachineIdentity, m.loc
}

// Location returns the server timezone. It never returns nil.
func (m *MachineInfo) Location() *time.Location {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// DiscoveredAt returns when the identity was last resolved (discovery or refresh).
func (m *MachineInfo) DiscoveredAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.discovered
}

// Refresh re-reads the identity fields. It keeps the
// previously resolved timezone when the server cannot be read this time around.
func (m *MachineInfo) Refresh(ctx context.Context, c *Client) error {
	sys, err := c.SystemInfo(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(sys.MachineName) == "" {
		return fmt.Errorf("duplicati: empty machine name in systeminfo")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.MachineName = strings.TrimSpace(sys.MachineName)
	m.Version = firstNonEmpty(sys.ServerVersionName, sys.ServerVersion)
	m.OSType = sys.OSType
	m.OSVersion = sys.OSVersion
	if loc, err := ResolveLocation(sys.ServerTimeZone, ""); err == nil {
		m.Timezone = sys.ServerTimeZone
		m.loc = loc
	}
	m.discovered = time.Now().UTC()
	return nil
}

var offsetInTZRe = regexp.MustCompile(`(?i)UTC\s*([+-])\s*(\d{1,2})(?::(\d{2}))?`)

// ResolveLocation turns a Duplicati timezone value into a *time.Location.
//
// Duplicati reports TimeZoneInfo.Local.Id, which is an IANA id on Unix and a
// Windows display id on Windows. As a fallback the location is derived from an
// explicit offset found in serverTime (e.g. "+02:00"), because a plain offset is
// still enough to place offset-less timestamps on the timeline.
func ResolveLocation(tz string, serverTime string) (*time.Location, error) {
	tz = strings.TrimSpace(tz)
	if tz != "" {
		if strings.EqualFold(tz, "UTC") || strings.EqualFold(tz, "Etc/UTC") || tz == "Z" {
			return time.UTC, nil
		}
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc, nil
		}
		// "(UTC+01:00) Amsterdam, Berlin, ..." style ids.
		if m := offsetInTZRe.FindStringSubmatch(tz); m != nil {
			sign := 1
			if m[1] == "-" {
				sign = -1
			}
			secs := sign * (toInt(m[2])*3600 + toInt(m[3])*60)
			return time.FixedZone(tz, secs), nil
		}
	}

	if secs, ok := offsetFromTimestamp(serverTime); ok {
		return time.FixedZone("UTC"+formatOffset(secs), secs), nil
	}

	return nil, fmt.Errorf("cannot resolve timezone %q", tz)
}

// offsetFromTimestamp extracts a UTC offset in seconds from an RFC3339 string.
func offsetFromTimestamp(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			_, off := t.Zone()
			return off, true
		}
	}
	return 0, false
}

func formatOffset(secs int) string {
	sign := "+"
	if secs < 0 {
		sign = "-"
		secs = -secs
	}
	return fmt.Sprintf("%s%02d:%02d", sign, secs/3600, (secs%3600)/60)
}

func toInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
