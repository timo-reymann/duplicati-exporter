package collector

import "github.com/prometheus/client_golang/prometheus"

// Machine label names, discovered from the Duplicati API itself.
const (
	labelMachineID   = "machine_id"
	labelMachineName = "machine_name"
	labelBackupName  = "backup_name"
	labelVersion     = "version"
	labelType        = "type"
)

var (
	machineLabels    = []string{labelMachineID, labelMachineName}
	backupLabels     = []string{labelMachineID, labelMachineName, labelBackupName}
	filesetLabels    = []string{labelMachineID, labelMachineName, labelBackupName, labelVersion}
	notificationLbls = []string{labelMachineID, labelMachineName, labelBackupName, labelType}
)

// descriptors holds every metric the exporter can emit.
type descriptors struct {
	// Machine level
	scrapeError     *prometheus.Desc
	up              *prometheus.Desc
	paused          *prometheus.Desc
	progressOverall *prometheus.Desc
	notifications   *prometheus.Desc
	info            *prometheus.Desc
	lastScrape      *prometheus.Desc
	scrapeDuration  *prometheus.Desc

	// Backup level: timing
	lastStarted     *prometheus.Desc
	lastFinished    *prometheus.Desc
	lastDuration    *prometheus.Desc
	lastDate        *prometheus.Desc
	nextRun         *prometheus.Desc
	scheduleLastRun *prometheus.Desc

	// Backup level: storage
	listCount        *prometheus.Desc
	totalQuota       *prometheus.Desc
	freeQuota        *prometheus.Desc
	assignedQuota    *prometheus.Desc
	targetFilesBytes *prometheus.Desc
	targetFilesCount *prometheus.Desc
	targetFilesets   *prometheus.Desc
	sourceFilesBytes *prometheus.Desc
	sourceFilesCount *prometheus.Desc

	// Backup level: other operations
	lastRestoreDuration *prometheus.Desc
	lastCompactTime     *prometheus.Desc
	lastVacuumTime      *prometheus.Desc
	lastSyncTime        *prometheus.Desc

	// Derived
	stale   *prometheus.Desc
	failure *prometheus.Desc

	// Filesets
	filesetsCount    *prometheus.Desc
	filesetTime      *prometheus.Desc
	filesetFileCount *prometheus.Desc
	filesetFileSizes *prometheus.Desc
	filesetIsFull    *prometheus.Desc

	// Webhook (last run)
	bytesUploaded   *prometheus.Desc
	bytesDownloaded *prometheus.Desc
	filesUploaded   *prometheus.Desc
	filesDownloaded *prometheus.Desc
	filesDeleted    *prometheus.Desc
	foldersCreated  *prometheus.Desc
	retryAttempts   *prometheus.Desc
	lastResult      *prometheus.Desc
	lastRunBegin    *prometheus.Desc
	lastRunEnd      *prometheus.Desc
	lastRunDuration *prometheus.Desc

	// Build
	buildInfo *prometheus.Desc
}

func newDescriptors() *descriptors {
	return &descriptors{
		scrapeError: prometheus.NewDesc(
			"duplicati_machine_scrape_error",
			"1 if the Duplicati API could not be reached for this machine during the last scrape, 0 otherwise.",
			machineLabels, nil),
		up: prometheus.NewDesc(
			"duplicati_machine_up",
			"1 if the Duplicati API is reachable and authenticated, 0 otherwise.",
			machineLabels, nil),
		paused: prometheus.NewDesc(
			"duplicati_machine_paused",
			"1 if the Duplicati scheduler is paused, 0 otherwise.",
			machineLabels, nil),
		progressOverall: prometheus.NewDesc(
			"duplicati_machine_progress_overall",
			"Overall progress of the currently running Duplicati task, between 0 and 1.",
			machineLabels, nil),
		notifications: prometheus.NewDesc(
			"duplicati_machine_notifications_count",
			"Number of open Duplicati notifications by type for a backup.",
			notificationLbls, nil),
		info: prometheus.NewDesc(
			"duplicati_machine_info",
			"Constant metric with machine metadata discovered from the Duplicati API.",
			[]string{labelMachineID, labelMachineName, "timezone", "version", "os_type", "os_version"}, nil),
		lastScrape: prometheus.NewDesc(
			"duplicati_machine_last_scrape_timestamp_seconds",
			"Unix timestamp of the last successful scrape of this machine, in UTC.",
			machineLabels, nil),
		scrapeDuration: prometheus.NewDesc(
			"duplicati_machine_scrape_duration_seconds",
			"Duration of the last scrape of this machine in seconds.",
			machineLabels, nil),

		lastStarted: prometheus.NewDesc(
			"duplicati_backup_last_started_time",
			"Unix timestamp in UTC of the start of the last completed backup run.",
			backupLabels, nil),
		lastFinished: prometheus.NewDesc(
			"duplicati_backup_last_finished_time",
			"Unix timestamp in UTC of the end of the last completed backup run.",
			backupLabels, nil),
		lastDuration: prometheus.NewDesc(
			"duplicati_backup_last_duration_seconds",
			"Duration in seconds of the last completed backup run.",
			backupLabels, nil),
		lastDate: prometheus.NewDesc(
			"duplicati_backup_last_date_time",
			"Unix timestamp in UTC of the newest remote fileset; advances only after a successful upload.",
			backupLabels, nil),
		nextRun: prometheus.NewDesc(
			"duplicati_backup_next_run_time",
			"Unix timestamp in UTC of the next scheduled run of this backup.",
			backupLabels, nil),
		scheduleLastRun: prometheus.NewDesc(
			"duplicati_backup_schedule_last_run_time",
			"Unix timestamp in UTC when the schedule of this backup last triggered a run.",
			backupLabels, nil),

		listCount: prometheus.NewDesc(
			"duplicati_backup_list_count",
			"Number of backups currently stored on the backend.",
			backupLabels, nil),
		totalQuota: prometheus.NewDesc(
			"duplicati_backup_total_quota_bytes",
			"Total quota space in bytes reported by the backend.",
			backupLabels, nil),
		freeQuota: prometheus.NewDesc(
			"duplicati_backup_free_quota_bytes",
			"Free quota space in bytes reported by the backend.",
			backupLabels, nil),
		assignedQuota: prometheus.NewDesc(
			"duplicati_backup_assigned_quota_bytes",
			"Assigned quota space in bytes reported by the backend.",
			backupLabels, nil),
		targetFilesBytes: prometheus.NewDesc(
			"duplicati_backup_target_files_bytes",
			"Size in bytes of the files stored on the backend.",
			backupLabels, nil),
		targetFilesCount: prometheus.NewDesc(
			"duplicati_backup_target_files_count",
			"Number of files stored on the backend.",
			backupLabels, nil),
		targetFilesets: prometheus.NewDesc(
			"duplicati_backup_target_filesets_count",
			"Number of filesets stored on the backend.",
			backupLabels, nil),
		sourceFilesBytes: prometheus.NewDesc(
			"duplicati_backup_source_files_bytes",
			"Size in bytes of the source files of this backup.",
			backupLabels, nil),
		sourceFilesCount: prometheus.NewDesc(
			"duplicati_backup_source_files_count",
			"Number of source files of this backup.",
			backupLabels, nil),

		lastRestoreDuration: prometheus.NewDesc(
			"duplicati_backup_last_restore_duration_seconds",
			"Duration in seconds of the last restore operation, when reported.",
			backupLabels, nil),
		lastCompactTime: prometheus.NewDesc(
			"duplicati_backup_last_compact_time",
			"Unix timestamp in UTC of the last compact operation, when reported.",
			backupLabels, nil),
		lastVacuumTime: prometheus.NewDesc(
			"duplicati_backup_last_vacuum_time",
			"Unix timestamp in UTC of the last vacuum operation, when reported.",
			backupLabels, nil),
		lastSyncTime: prometheus.NewDesc(
			"duplicati_backup_last_sync_time",
			"Unix timestamp in UTC of the last sync operation, when reported.",
			backupLabels, nil),

		stale: prometheus.NewDesc(
			"duplicati_backup_stale",
			"1 if the next scheduled run time has passed without a finished or successful backup since, 0 otherwise.",
			backupLabels, nil),
		failure: prometheus.NewDesc(
			"duplicati_backup_failure",
			"1 if an active error notification exists for this backup, 0 otherwise.",
			backupLabels, nil),

		filesetsCount: prometheus.NewDesc(
			"duplicati_backup_filesets_count",
			"Number of stored versions of this backup.",
			backupLabels, nil),
		filesetTime: prometheus.NewDesc(
			"duplicati_backup_fileset_time",
			"Unix timestamp in UTC of a stored backup version.",
			filesetLabels, nil),
		filesetFileCount: prometheus.NewDesc(
			"duplicati_backup_fileset_file_count",
			"Number of files in a stored backup version.",
			filesetLabels, nil),
		filesetFileSizes: prometheus.NewDesc(
			"duplicati_backup_fileset_file_sizes",
			"Size in bytes of a stored backup version.",
			filesetLabels, nil),
		filesetIsFull: prometheus.NewDesc(
			"duplicati_backup_fileset_is_full",
			"1 if the stored backup version is a full backup, 0 if incremental.",
			filesetLabels, nil),

		bytesUploaded: prometheus.NewDesc(
			"duplicati_backup_bytes_uploaded_total",
			"Bytes uploaded to the backend by the last reported run.",
			backupLabels, nil),
		bytesDownloaded: prometheus.NewDesc(
			"duplicati_backup_bytes_downloaded_total",
			"Bytes downloaded from the backend by the last reported run.",
			backupLabels, nil),
		filesUploaded: prometheus.NewDesc(
			"duplicati_backup_files_uploaded_total",
			"Files uploaded to the backend by the last reported run.",
			backupLabels, nil),
		filesDownloaded: prometheus.NewDesc(
			"duplicati_backup_files_downloaded_total",
			"Files downloaded from the backend by the last reported run.",
			backupLabels, nil),
		filesDeleted: prometheus.NewDesc(
			"duplicati_backup_files_deleted_total",
			"Files deleted on the backend by the last reported run.",
			backupLabels, nil),
		foldersCreated: prometheus.NewDesc(
			"duplicati_backup_folders_created_total",
			"Folders created on the backend by the last reported run.",
			backupLabels, nil),
		retryAttempts: prometheus.NewDesc(
			"duplicati_backup_retry_attempts",
			"Retry attempts made by the last reported run.",
			backupLabels, nil),
		lastResult: prometheus.NewDesc(
			"duplicati_backup_last_result",
			"Parsed result of the last reported run: 0 Success, 1 Warning, 2 Fatal, 3 Unknown.",
			backupLabels, nil),
		lastRunBegin: prometheus.NewDesc(
			"duplicati_backup_last_run_begin_time",
			"Unix timestamp in UTC when the last reported run began.",
			backupLabels, nil),
		lastRunEnd: prometheus.NewDesc(
			"duplicati_backup_last_run_end_time",
			"Unix timestamp in UTC when the last reported run ended.",
			backupLabels, nil),
		lastRunDuration: prometheus.NewDesc(
			"duplicati_backup_last_run_duration_seconds",
			"Duration in seconds of the last reported run.",
			backupLabels, nil),

		buildInfo: prometheus.NewDesc(
			"duplicati_build_info",
			"Build information of the duplicati-exporter binary.",
			[]string{"version", "revision", "goversion"}, nil),
	}
}
