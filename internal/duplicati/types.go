// Package duplicati implements a client for the Duplicati Server HTTP API (v1).
package duplicati

import (
	"encoding/json"
	"strings"
)

// SystemInfo is the subset of GET /api/v1/systeminfo the exporter needs.
type SystemInfo struct {
	APIVersion        int    `json:"APIVersion"`
	ServerVersion     string `json:"ServerVersion"`
	ServerVersionName string `json:"ServerVersionName"`
	ServerVersionType string `json:"ServerVersionType"`
	ServerTimeZone    string `json:"ServerTimeZone"`
	MachineName       string `json:"MachineName"`
	OSType            string `json:"OSType"`
	OSVersion         string `json:"OSVersion"`
}

// ServerState is the response of GET /api/v1/serverstate. Tuple-valued fields
// are kept as raw JSON because Duplicati serializes their shape differently
// across versions, and the exporter only consumes ProgramState.
type ServerState struct {
	ActiveTask          json.RawMessage `json:"ActiveTask"`
	ProgramState        string          `json:"ProgramState"`
	SchedulerQueueIDs   json.RawMessage `json:"SchedulerQueueIds"`
	ProposedSchedule    json.RawMessage `json:"ProposedSchedule"`
	HasWarning          bool            `json:"HasWarning"`
	HasError            bool            `json:"HasError"`
	SuggestedStatusIcon string          `json:"SuggestedStatusIcon"`
	EstimatedPauseEnd   string          `json:"EstimatedPauseEnd"`
	LastEventID         int64           `json:"LastEventID"`
	LastDataUpdateID    int64           `json:"LastDataUpdateID"`
	LastNotificationID  int64           `json:"LastNotificationUpdateID"`
	UpdatedVersion      *string         `json:"UpdatedVersion"`
	UpdaterState        string          `json:"UpdaterState"`
	UpdateDownloadLink  *string         `json:"UpdateDownloadLink"`
	UpdateProgress      float64         `json:"UpdateDownloadProgress"`
	Type                string          `json:"Type"`
}

// Paused reports whether the scheduler is currently paused.
func (s *ServerState) Paused() bool {
	return strings.EqualFold(s.ProgramState, "Paused")
}

// Backup is the backup configuration DTO (GET /api/v1/backups, /backup/{id}).
type Backup struct {
	ID            string            `json:"ID"`
	ExternalID    string            `json:"ExternalID"`
	Name          string            `json:"Name"`
	Description   string            `json:"Description"`
	Tags          []string          `json:"Tags"`
	TargetURL     string            `json:"TargetURL"`
	DBPath        string            `json:"DBPath"`
	DBPathExists  bool              `json:"DBPathExists"`
	Sources       []string          `json:"Sources"`
	Settings      []Setting         `json:"Settings"`
	Metadata      map[string]string `json:"Metadata"`
	OperationType string            `json:"OperationType"`
	IsTemporary   bool              `json:"IsTemporary"`
}

// Setting is a single backup option as returned by the API.
type Setting struct {
	Filter string `json:"Filter"`
	Name   string `json:"Name"`
	Value  string `json:"Value"`
}

// Option returns the value of a backup option, tolerating a leading "--".
func (b *Backup) Option(name string) string {
	for _, s := range b.Settings {
		n := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s.Name)), "--")
		if n == name {
			return strings.TrimSpace(s.Value)
		}
	}
	return ""
}

// Schedule describes when a backup is expected to run.
type Schedule struct {
	ID          int64    `json:"ID"`
	Tags        []string `json:"Tags"`
	Time        string   `json:"Time"`
	Repeat      string   `json:"Repeat"`
	LastRun     string   `json:"LastRun"`
	Rule        string   `json:"Rule"`
	AllowedDays []string `json:"AllowedDays"`
}

// BackupWithSchedule is one element of GET /api/v1/backups.
type BackupWithSchedule struct {
	Backup   Backup    `json:"Backup"`
	Schedule *Schedule `json:"Schedule"`
}

// BackupDetail is the response of GET /api/v1/backup/{id}.
type BackupDetail struct {
	Backup       Backup            `json:"Backup"`
	Schedule     *Schedule         `json:"Schedule"`
	DisplayNames map[string]string `json:"DisplayNames"`
}

// Fileset is one stored version as returned by GET /api/v1/backup/{id}/filesets.
type Fileset struct {
	Version      int64  `json:"Version"`
	IsFullBackup int    `json:"IsFullBackup"`
	Time         string `json:"Time"`
	FileCount    int64  `json:"FileCount"`
	FileSizes    int64  `json:"FileSizes"`
	Label        string `json:"Label"`
}

// Notification is one entry of GET /api/v1/notifications.
type Notification struct {
	ID         int64  `json:"ID"`
	Type       string `json:"Type"`
	Title      string `json:"Title"`
	Message    string `json:"Message"`
	Exception  string `json:"Exception"`
	BackupID   string `json:"BackupID"`
	Action     string `json:"Action"`
	Timestamp  string `json:"Timestamp"`
	LogEntryID string `json:"LogEntryID"`
	MessageID  string `json:"MessageID"`
}

// IsError reports whether the notification signals a failure.
func (n *Notification) IsError() bool { return strings.EqualFold(n.Type, "Error") }

// IsWarning reports whether the notification signals a warning.
func (n *Notification) IsWarning() bool { return strings.EqualFold(n.Type, "Warning") }

// ProgressState is the response of GET /api/v1/progressstate. It only exists
// while a task is running, otherwise Duplicati answers 404.
type ProgressState struct {
	BackupID           *string `json:"BackupID"`
	TaskID             int64   `json:"TaskID"`
	BackendAction      string  `json:"BackendAction"`
	BackendPath        string  `json:"BackendPath"`
	Phase              string  `json:"Phase"`
	OverallProgress    float64 `json:"OverallProgress"`
	ProcessedFileCount int64   `json:"ProcessedFileCount"`
	ProcessedFileSize  int64   `json:"ProcessedFileSize"`
	TotalFileCount     int64   `json:"TotalFileCount"`
	TotalFileSize      int64   `json:"TotalFileSize"`
	StillCounting      bool    `json:"StillCounting"`
	CurrentFilename    string  `json:"CurrentFilename"`
}

// Progress reports whether a task is currently running.
func (p *ProgressState) Running() bool { return p != nil && p.TaskID > 0 }

// Metadata key names used by Duplicati's backup metadata dictionary.
const (
	MetaLastBackupStarted  = "LastBackupStarted"
	MetaLastBackupFinished = "LastBackupFinished"
	MetaLastBackupDuration = "LastBackupDuration"
	MetaLastBackupDate     = "LastBackupDate"

	MetaLastRestoreStarted  = "LastRestoreStarted"
	MetaLastRestoreFinished = "LastRestoreFinished"
	MetaLastRestoreDuration = "LastRestoreDuration"
	MetaLastCompactStarted  = "LastCompactStarted"
	MetaLastCompactFinished = "LastCompactFinished"
	MetaLastVacuumStarted   = "LastVacuumStarted"
	MetaLastVacuumFinished  = "LastVacuumFinished"
	MetaLastSyncStarted     = "LastSyncStarted"
	MetaLastSyncFinished    = "LastSyncFinished"
	MetaLastSyncDuration    = "LastSyncDuration"

	MetaSourceFilesSize  = "SourceFilesSize"
	MetaSourceFilesCount = "SourceFilesCount"
	MetaTargetFilesSize  = "TargetFilesSize"
	MetaTargetFilesCount = "TargetFilesCount"
	MetaTargetFilesets   = "TargetFilesetsCount"
	MetaKnownFileCount   = "KnownFileCount"
	MetaKnownFileSize    = "KnownFileSize"
	MetaBackupListCount  = "BackupListCount"
	MetaTotalQuotaSpace  = "TotalQuotaSpace"
	MetaFreeQuotaSpace   = "FreeQuotaSpace"
	MetaAssignedQuota    = "AssignedQuotaSpace"
)
