// Package collector turns Duplicati API responses into Prometheus metrics.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/timo-reymann/duplicati-exporter/internal/buildinfo"
	"github.com/timo-reymann/duplicati-exporter/internal/duplicati"
	"github.com/timo-reymann/duplicati-exporter/internal/webhook"
)

// ErrAllMachinesDown is returned by Poll when every configured machine failed,
// which makes /metrics answer with HTTP 500 so Prometheus marks the target down.
var ErrAllMachinesDown = errors.New("collector: all Duplicati machines are unreachable")

// filesetFetchBudget bounds how long one machine may spend listing versions.
const filesetFetchBudget = 30 * time.Second

// Target is one Duplicati endpoint the exporter polls.
type Target struct {
	// Name is only used for logs.
	Name    string
	Client  *duplicati.Client
	Machine *duplicati.MachineInfo
}

// Options configures an Exporter.
type Options struct {
	Targets          []*Target
	Webhooks         *webhook.Store
	Timeout          time.Duration
	FilesetsEnabled  bool
	FilesetsCacheTTL time.Duration
	// IdentityRefreshInterval is how often the machine identity (systeminfo, a
	// large payload) is re-read. Defaults to 15 minutes.
	IdentityRefreshInterval time.Duration
	Logger                  *slog.Logger
	Build                   buildinfo.Info
}

// Exporter polls Duplicati and exposes the resulting metrics. It implements
// prometheus.Collector; Poll must be called before Collect to refresh the data.
type Exporter struct {
	opts Options
	desc *descriptors

	mu    sync.RWMutex
	snaps []*machineSnapshot

	// pollMu serializes polls; pollSeq counts completed polls so callers that
	// waited on an in-flight poll can reuse its result instead of polling again.
	pollMu      sync.Mutex
	pollSeq     atomic.Uint64
	lastPollErr error

	filesetMu sync.Mutex
	filesets  map[string]filesetCacheEntry
}

type filesetCacheEntry struct {
	at       time.Time
	filesets []duplicati.Fileset
	err      error
}

// machineSnapshot is the polled state of one machine.
type machineSnapshot struct {
	target *Target

	scrapeErr  float64
	up         float64
	paused     float64
	progress   float64
	scrapeTook time.Duration
	scrapedAt  time.Time

	identity duplicati.MachineIdentity
	loc      *time.Location

	serverState   *duplicati.ServerState
	backups       []backupState
	notifications []duplicati.Notification
	notifsOK      bool
}

// backupState is the polled state of one backup configuration.
type backupState struct {
	id            string
	name          string
	operationType string
	schedule      *duplicati.Schedule
	meta          map[string]string
	filesets      []duplicati.Fileset
	filesetsOK    bool
	webhook       *webhook.Report
}

// New builds an Exporter.
func New(opts Options) *Exporter {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.IdentityRefreshInterval <= 0 {
		opts.IdentityRefreshInterval = 15 * time.Minute
	}
	if opts.Webhooks == nil {
		opts.Webhooks = webhook.NewStore()
	}
	return &Exporter{
		opts:     opts,
		desc:     newDescriptors(),
		filesets: make(map[string]filesetCacheEntry),
	}
}

// Poll refreshes the state of every configured machine. It returns
// ErrAllMachinesDown when no machine could be read.
//
// Concurrent callers share one poll: a caller that had to wait for an in-flight
// poll returns that poll's result. The poll is detached from ctx cancelation
// (bounded by the per-machine timeout instead) so a scraper that gives up early
// cannot leave the machines marked as down.
func (e *Exporter) Poll(ctx context.Context) error {
	seen := e.pollSeq.Load()
	e.pollMu.Lock()
	defer e.pollMu.Unlock()
	if e.pollSeq.Load() != seen {
		return e.lastPollErr
	}

	err := e.poll(context.WithoutCancel(ctx))
	e.lastPollErr = err
	e.pollSeq.Add(1)
	return err
}

func (e *Exporter) poll(ctx context.Context) error {
	type result struct {
		idx int
		s   *machineSnapshot
	}

	results := make([]result, len(e.opts.Targets))
	var wg sync.WaitGroup
	for i, t := range e.opts.Targets {
		wg.Add(1)
		go func(idx int, target *Target) {
			defer wg.Done()
			results[idx] = result{idx: idx, s: e.pollMachine(ctx, target)}
		}(i, t)
	}
	wg.Wait()

	snaps := make([]*machineSnapshot, 0, len(results))
	down := 0
	for _, r := range results {
		if r.s == nil {
			continue
		}
		if r.s.scrapeErr == 1 {
			down++
		}
		snaps = append(snaps, r.s)
	}

	e.mu.Lock()
	e.snaps = snaps
	e.mu.Unlock()

	if len(e.opts.Targets) > 0 && down == len(e.opts.Targets) {
		return ErrAllMachinesDown
	}
	return nil
}

// pollMachine reads one machine and merges the result with the previous
// snapshot so that a failed scrape keeps serving the last known backup data.
func (e *Exporter) pollMachine(ctx context.Context, target *Target) *machineSnapshot {
	identity, loc := target.Machine.Snapshot()
	snap := &machineSnapshot{
		target:   target,
		identity: identity,
		loc:      loc,
		up:       1,
	}

	start := time.Now()
	defer func() { snap.scrapeTook = time.Since(start); snap.scrapedAt = time.Now().UTC() }()

	reqCtx, cancel := context.WithTimeout(ctx, e.opts.Timeout)
	defer cancel()

	// Core data. A failing serverstate means the API is unreachable or rejected
	// us, so the machine counts as down; a failing backup list only makes the
	// scrape incomplete and is reported via duplicati_machine_scrape_error.
	state, stateErr := target.Client.ServerState(reqCtx)
	backups, backupsErr := target.Client.Backups(reqCtx)
	if stateErr != nil {
		e.opts.Logger.Debug("serverstate failed",
			slog.String("target", target.Name), slog.String("machine", identity.MachineName), slog.Any("err", stateErr))
		snap.scrapeErr = 1
		snap.up = 0
		e.mergePrevious(snap)
		return snap
	}
	snap.serverState = state
	snap.paused = boolToFloat(state.Paused())

	if backupsErr != nil {
		e.opts.Logger.Debug("backups failed",
			slog.String("target", target.Name), slog.String("machine", identity.MachineName), slog.Any("err", backupsErr))
		snap.scrapeErr = 1
		e.mergePrevious(snap)
		return snap
	}

	// Best effort data: a failure here must not mark the machine as down.
	if notes, err := target.Client.Notifications(reqCtx); err == nil {
		snap.notifications = notes
		snap.notifsOK = true
	} else {
		e.opts.Logger.Debug("notifications failed",
			slog.String("target", target.Name), slog.String("machine", identity.MachineName), slog.Any("err", err))
	}
	if prog, err := target.Client.ProgressState(reqCtx); err == nil && prog.Running() {
		snap.progress = clamp01(prog.OverallProgress)
	} else if err != nil && !errors.Is(err, duplicati.ErrNotFound) {
		e.opts.Logger.Debug("progressstate failed",
			slog.String("target", target.Name), slog.String("machine", identity.MachineName), slog.Any("err", err))
	}

	// Re-read the identity only when it is due: systeminfo is by far the
	// largest API response and rarely changes.
	if time.Since(target.Machine.DiscoveredAt()) >= e.opts.IdentityRefreshInterval {
		if err := target.Machine.Refresh(reqCtx, target.Client); err != nil {
			e.opts.Logger.Debug("identity refresh failed",
				slog.String("target", target.Name), slog.Any("err", err))
		} else {
			identity, loc = target.Machine.Snapshot()
			snap.identity = identity
			snap.loc = loc
		}
	}

	for _, b := range backups {
		snap.backups = append(snap.backups, backupState{
			id:            b.Backup.ID,
			name:          b.Backup.Name,
			operationType: b.Backup.OperationType,
			schedule:      b.Schedule,
			meta:          b.Backup.Metadata,
			webhook:       e.lookupWebhook(identity, b.Backup),
		})
	}

	e.collectFilesets(reqCtx, snap)

	return snap
}

// mergePrevious copies the last known good backup data into a failed snapshot so
// a temporary outage does not blank out the dashboard.
func (e *Exporter) mergePrevious(snap *machineSnapshot) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, prev := range e.snaps {
		if prev.target != snap.target {
			continue
		}
		if prev.scrapeErr == 1 {
			return
		}
		snap.backups = prev.backups
		snap.notifications = prev.notifications
		snap.notifsOK = prev.notifsOK
		snap.serverState = prev.serverState
		snap.paused = prev.paused
		snap.progress = prev.progress
		return
	}
}

func (e *Exporter) lookupWebhook(id duplicati.MachineIdentity, b duplicati.Backup) *webhook.Report {
	r, ok := e.opts.Webhooks.Lookup(id.MachineID, id.MachineName, b.Name)
	if !ok {
		return nil
	}
	return r
}

// collectFilesets lists the stored versions of every backup, with a per-machine
// time budget and an optional cache so repeated scrapes do not hammer the backend.
func (e *Exporter) collectFilesets(ctx context.Context, snap *machineSnapshot) {
	if !e.opts.FilesetsEnabled || len(snap.backups) == 0 {
		return
	}

	budgetCtx, cancel := context.WithTimeout(ctx, filesetFetchBudget)
	defer cancel()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range snap.backups {
		wg.Add(1)
		go func(b *backupState) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			key := snap.identity.MachineID + "|" + b.id
			if fs, hit, ok := e.cachedFilesets(key); hit {
				b.filesets, b.filesetsOK = fs, ok
				return
			}

			fs, err := snap.target.Client.Filesets(budgetCtx, b.id)
			if err != nil {
				e.opts.Logger.Debug("filesets failed",
					slog.String("machine", snap.identity.MachineName),
					slog.String("backup", b.name), slog.Any("err", err))
				e.storeFilesets(key, nil, false)
				return
			}
			b.filesets, b.filesetsOK = fs, true
			e.storeFilesets(key, fs, true)
		}(&snap.backups[i])
	}
	wg.Wait()
}

// filesetFailureTTL bounds how long a failed listing is remembered, so a broken
// backend is not retried on every scrape.
const filesetFailureTTL = time.Minute

// cachedFilesets returns the cached listing for key. hit reports whether the
// entry is usable (including a remembered failure); ok is false for a failure.
func (e *Exporter) cachedFilesets(key string) (fs []duplicati.Fileset, hit, ok bool) {
	ttl := e.opts.FilesetsCacheTTL
	if ttl <= 0 {
		return nil, false, false
	}
	e.filesetMu.Lock()
	defer e.filesetMu.Unlock()
	entry, found := e.filesets[key]
	if !found {
		return nil, false, false
	}
	if entry.err != nil {
		ttl = min(ttl, filesetFailureTTL)
	}
	if time.Since(entry.at) > ttl {
		return nil, false, false
	}
	return entry.filesets, true, entry.err == nil
}

func (e *Exporter) storeFilesets(key string, fs []duplicati.Fileset, ok bool) {
	if e.opts.FilesetsCacheTTL <= 0 {
		return
	}
	e.filesetMu.Lock()
	defer e.filesetMu.Unlock()
	var err error
	if !ok {
		err = errors.New("fileset listing failed")
	}
	e.filesets[key] = filesetCacheEntry{at: time.Now(), filesets: fs, err: err}
}

// Describe implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	d := e.desc
	for _, desc := range []*prometheus.Desc{
		d.scrapeError, d.up, d.paused, d.progressOverall, d.notifications, d.info,
		d.lastScrape, d.scrapeDuration,
		d.lastStarted, d.lastFinished, d.lastDuration, d.lastDate, d.nextRun, d.scheduleLastRun,
		d.listCount, d.totalQuota, d.freeQuota, d.assignedQuota,
		d.targetFilesBytes, d.targetFilesCount, d.targetFilesets,
		d.sourceFilesBytes, d.sourceFilesCount,
		d.lastRestoreDuration, d.lastCompactTime, d.lastVacuumTime, d.lastSyncTime,
		d.stale, d.failure,
		d.filesetsCount, d.filesetTime, d.filesetFileCount, d.filesetFileSizes, d.filesetIsFull,
		d.bytesUploaded, d.bytesDownloaded, d.filesUploaded, d.filesDownloaded,
		d.filesDeleted, d.foldersCreated, d.retryAttempts, d.lastResult,
		d.lastRunBegin, d.lastRunEnd, d.lastRunDuration,
		d.buildInfo,
	} {
		ch <- desc
	}
}

// Collect implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mu.RLock()
	snaps := make([]*machineSnapshot, len(e.snaps))
	copy(snaps, e.snaps)
	e.mu.RUnlock()

	for _, snap := range snaps {
		e.collectMachine(ch, snap)
	}

	b := e.opts.Build
	ch <- prometheus.MustNewConstMetric(e.desc.buildInfo, prometheus.GaugeValue, 1,
		b.Version, b.Revision, b.GoVersion)
}

func (e *Exporter) collectMachine(ch chan<- prometheus.Metric, snap *machineSnapshot) {
	d := e.desc
	id, name := snap.identity.MachineID, snap.identity.MachineName
	now := time.Now().UTC()

	ch <- prometheus.MustNewConstMetric(d.scrapeError, prometheus.GaugeValue, snap.scrapeErr, id, name)
	ch <- prometheus.MustNewConstMetric(d.up, prometheus.GaugeValue, snap.up, id, name)
	ch <- prometheus.MustNewConstMetric(d.paused, prometheus.GaugeValue, snap.paused, id, name)
	ch <- prometheus.MustNewConstMetric(d.progressOverall, prometheus.GaugeValue, snap.progress, id, name)
	ch <- prometheus.MustNewConstMetric(d.info, prometheus.GaugeValue, 1,
		id, name, snap.identity.Timezone, snap.identity.Version, snap.identity.OSType, snap.identity.OSVersion)
	if !snap.scrapedAt.IsZero() {
		ch <- prometheus.MustNewConstMetric(d.lastScrape, prometheus.GaugeValue,
			float64(snap.scrapedAt.Unix()), id, name)
	}
	ch <- prometheus.MustNewConstMetric(d.scrapeDuration, prometheus.GaugeValue,
		snap.scrapeTook.Seconds(), id, name)

	if snap.notifsOK {
		e.collectNotifications(ch, snap)
	}

	for i := range snap.backups {
		e.collectBackup(ch, snap, &snap.backups[i], now)
	}
}

func (e *Exporter) collectNotifications(ch chan<- prometheus.Metric, snap *machineSnapshot) {
	d := e.desc
	id, name := snap.identity.MachineID, snap.identity.MachineName

	// Re-key by backup name so the label matches the backup metrics.
	byName := map[string]map[string]float64{}
	for _, b := range snap.backups {
		byName[b.id] = map[string]float64{}
	}
	for _, n := range snap.notifications {
		m, ok := byName[n.BackupID]
		if !ok {
			continue
		}
		m[n.Type]++
	}

	for _, b := range snap.backups {
		seen := map[string]bool{}
		for _, n := range snap.notifications {
			if n.BackupID != b.id || seen[n.Type] {
				continue
			}
			seen[n.Type] = true
			ch <- prometheus.MustNewConstMetric(d.notifications, prometheus.GaugeValue,
				byName[b.id][n.Type], id, name, b.name, n.Type)
		}
	}
}

func (e *Exporter) collectBackup(ch chan<- prometheus.Metric, snap *machineSnapshot, b *backupState, now time.Time) {
	d := e.desc
	id, name := snap.identity.MachineID, snap.identity.MachineName
	loc := snap.loc
	meta := b.meta
	if meta == nil {
		meta = map[string]string{}
	}

	setTime := func(desc *prometheus.Desc, raw string) {
		if v := duplicati.ParseTimestampUnix(raw, loc); v > 0 {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, id, name, b.name)
		}
	}
	setGauge := func(desc *prometheus.Desc, raw string) {
		if raw == "" {
			return
		}
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, duplicati.ParseInt64(raw), id, name, b.name)
	}
	setDuration := func(desc *prometheus.Desc, raw string) {
		if raw == "" {
			return
		}
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, duplicati.ParseDurationSeconds(raw), id, name, b.name)
	}

	// Timing
	setTime(d.lastStarted, meta[startedKey(b.operationType)])
	setTime(d.lastFinished, meta[finishedKey(b.operationType)])
	setDuration(d.lastDuration, meta[durationKey(b.operationType)])
	setTime(d.lastDate, meta[duplicati.MetaLastBackupDate])

	// Schedule
	nextRun := time.Time{}
	if b.schedule != nil {
		nextRun, _ = duplicati.ParseTimestamp(b.schedule.Time, loc)
		if !nextRun.IsZero() {
			ch <- prometheus.MustNewConstMetric(d.nextRun, prometheus.GaugeValue,
				float64(nextRun.Unix()), id, name, b.name)
		}
		if b.schedule.LastRun != "" {
			if v := duplicati.ParseTimestampUnix(b.schedule.LastRun, loc); v > 0 {
				ch <- prometheus.MustNewConstMetric(d.scheduleLastRun, prometheus.GaugeValue, v, id, name, b.name)
			}
		}
	}

	// Storage
	setGauge(d.listCount, meta[duplicati.MetaBackupListCount])
	setGauge(d.totalQuota, meta[duplicati.MetaTotalQuotaSpace])
	setGauge(d.freeQuota, meta[duplicati.MetaFreeQuotaSpace])
	setGauge(d.assignedQuota, meta[duplicati.MetaAssignedQuota])
	setGauge(d.targetFilesBytes, meta[duplicati.MetaTargetFilesSize])
	setGauge(d.targetFilesCount, meta[duplicati.MetaTargetFilesCount])
	setGauge(d.targetFilesets, meta[duplicati.MetaTargetFilesets])
	setGauge(d.sourceFilesBytes, meta[duplicati.MetaSourceFilesSize])
	setGauge(d.sourceFilesCount, meta[duplicati.MetaSourceFilesCount])

	// Other operations
	setDuration(d.lastRestoreDuration, meta[duplicati.MetaLastRestoreDuration])
	setTime(d.lastCompactTime, firstNonEmpty(meta[duplicati.MetaLastCompactFinished], meta[duplicati.MetaLastCompactStarted]))
	setTime(d.lastVacuumTime, firstNonEmpty(meta[duplicati.MetaLastVacuumFinished], meta[duplicati.MetaLastVacuumStarted]))
	setTime(d.lastSyncTime, firstNonEmpty(meta[duplicati.MetaLastSyncFinished], meta[duplicati.MetaLastSyncStarted]))

	// Derived: staleness against Duplicati's own schedule.
	if !nextRun.IsZero() && now.After(nextRun) {
		lastFinished, _ := duplicati.ParseTimestamp(meta[finishedKey(b.operationType)], loc)
		lastDate, _ := duplicati.ParseTimestamp(meta[duplicati.MetaLastBackupDate], loc)
		// lastDate marks the last successful upload; when Duplicati did not
		// report it, only the finish time can be judged.
		if lastFinished.Before(nextRun) || (!lastDate.IsZero() && lastDate.Before(nextRun)) {
			ch <- prometheus.MustNewConstMetric(d.stale, prometheus.GaugeValue, 1, id, name, b.name)
		} else {
			ch <- prometheus.MustNewConstMetric(d.stale, prometheus.GaugeValue, 0, id, name, b.name)
		}
	} else if b.schedule != nil && b.schedule.Time != "" {
		ch <- prometheus.MustNewConstMetric(d.stale, prometheus.GaugeValue, 0, id, name, b.name)
	}

	// Derived: active error notification for this backup.
	if snap.notifsOK {
		failed := 0.0
		for _, n := range snap.notifications {
			if n.BackupID == b.id && n.IsError() {
				failed = 1
				break
			}
		}
		ch <- prometheus.MustNewConstMetric(d.failure, prometheus.GaugeValue, failed, id, name, b.name)
	}

	// Filesets
	if b.filesetsOK {
		ch <- prometheus.MustNewConstMetric(d.filesetsCount, prometheus.GaugeValue,
			float64(len(b.filesets)), id, name, b.name)
		for _, fs := range b.filesets {
			v := fmt.Sprintf("%d", fs.Version)
			if ts := duplicati.ParseTimestampUnix(fs.Time, loc); ts > 0 {
				ch <- prometheus.MustNewConstMetric(d.filesetTime, prometheus.GaugeValue, ts, id, name, b.name, v)
			}
			ch <- prometheus.MustNewConstMetric(d.filesetFileCount, prometheus.GaugeValue,
				float64(fs.FileCount), id, name, b.name, v)
			ch <- prometheus.MustNewConstMetric(d.filesetFileSizes, prometheus.GaugeValue,
				float64(fs.FileSizes), id, name, b.name, v)
			ch <- prometheus.MustNewConstMetric(d.filesetIsFull, prometheus.GaugeValue,
				boolToFloat(fs.IsFullBackup > 0), id, name, b.name, v)
		}
	}

	// Webhook (last reported run)
	if r := b.webhook; r != nil {
		ch <- prometheus.MustNewConstMetric(d.bytesUploaded, prometheus.GaugeValue, float64(r.BytesUploaded), id, name, b.name)
		ch <- prometheus.MustNewConstMetric(d.bytesDownloaded, prometheus.GaugeValue, float64(r.BytesDownloaded), id, name, b.name)
		ch <- prometheus.MustNewConstMetric(d.filesUploaded, prometheus.GaugeValue, float64(r.FilesUploaded), id, name, b.name)
		ch <- prometheus.MustNewConstMetric(d.filesDownloaded, prometheus.GaugeValue, float64(r.FilesDownloaded), id, name, b.name)
		ch <- prometheus.MustNewConstMetric(d.filesDeleted, prometheus.GaugeValue, float64(r.FilesDeleted), id, name, b.name)
		ch <- prometheus.MustNewConstMetric(d.foldersCreated, prometheus.GaugeValue, float64(r.FoldersCreated), id, name, b.name)
		ch <- prometheus.MustNewConstMetric(d.retryAttempts, prometheus.GaugeValue, float64(r.RetryAttempts), id, name, b.name)
		ch <- prometheus.MustNewConstMetric(d.lastResult, prometheus.GaugeValue,
			resultValue(r.ParsedResult), id, name, b.name)
		if !r.BeginTime.IsZero() {
			ch <- prometheus.MustNewConstMetric(d.lastRunBegin, prometheus.GaugeValue,
				float64(r.BeginTime.Unix()), id, name, b.name)
		}
		if !r.EndTime.IsZero() {
			ch <- prometheus.MustNewConstMetric(d.lastRunEnd, prometheus.GaugeValue,
				float64(r.EndTime.Unix()), id, name, b.name)
		}
		if r.Duration > 0 {
			ch <- prometheus.MustNewConstMetric(d.lastRunDuration, prometheus.GaugeValue,
				r.Duration.Seconds(), id, name, b.name)
		}
	}
}

func startedKey(operationType string) string {
	if strings.EqualFold(operationType, "Sync") {
		return duplicati.MetaLastSyncStarted
	}
	return duplicati.MetaLastBackupStarted
}

func finishedKey(operationType string) string {
	if strings.EqualFold(operationType, "Sync") {
		return duplicati.MetaLastSyncFinished
	}
	return duplicati.MetaLastBackupFinished
}

func durationKey(operationType string) string {
	if strings.EqualFold(operationType, "Sync") {
		return duplicati.MetaLastSyncDuration
	}
	return duplicati.MetaLastBackupDuration
}

func resultValue(result string) float64 {
	switch {
	case strings.EqualFold(result, "Success"):
		return 0
	case strings.EqualFold(result, "Warning"):
		return 1
	case strings.EqualFold(result, "Fatal"), strings.EqualFold(result, "Error"):
		return 2
	default:
		return 3
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
