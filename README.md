# duplicati-exporter

[![CircleCI](https://dl.circleci.com/status-badge/img/gh/timo-reymann/duplicati-exporter/tree/main.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/timo-reymann/duplicati-exporter/tree/main)
[![GitHub Release](https://img.shields.io/github/v/release/timo-reymann/duplicati-exporter)](https://github.com/timo-reymann/duplicati-exporter/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A Prometheus exporter for [Duplicati](https://duplicati.com) that polls the Duplicati
Server API on every scrape and exposes backup health, schedule, storage and fileset
metrics.

## Features

- **Pull-based, stateless** — polls the Duplicati HTTP API inside the `/metrics`
  handler. No background timers, no internal cache drift, restarts are trivially safe.
- **Multi-machine** — scrape several Duplicati instances from a single exporter.
  Each machine is polled independently; one machine being down does not affect
  metrics from the others.
- **Staleness detection** — Duplicati's own schedule (`NextRun`) is compared with the
  last finished and last successful (newest remote fileset) backups, so alerts can be
  based on *freshness* rather than a single fail flag.
- **Timezone aware** — every timestamp is exposed as a UTC Unix epoch (seconds), even
  for API fields that Duplicati renders in the server's configured timezone.
- **Webhook ingestion (optional)** — accepts Duplicati `send-http` reports on
  `/report` for per-run transfer deltas, `ParsedResult` and error/warning text, which
  are not available through the API.
- **Durable by design** — the API is the source of truth, and Duplicati persists the
  metadata in its own database. A restart of the exporter (or a webhook that arrives
  while the exporter is down) does not lose history.

## Installation

### Binary

Download the latest release for your platform from the
[releases page](https://github.com/timo-reymann/duplicati-exporter/releases):

```console
$ ./duplicati-exporter --help
```

### Docker

```console
$ docker run -p 9685:9685 timoreymann/duplicati-exporter:latest \
    --server 'name=primary;url=https://duplicati.example.com:8200;password=changeme'
```

### Build from source

```console
$ make build
$ ./dist/duplicati-exporter_linux-amd64 --help
```

## Configuration

The exporter is configured with flags (environment variables are supported for
containers, see below).

| Flag | Default | Description |
|---|---|---|
| `--web.listen-address` | `:9685` | Address to listen on for HTTP requests |
| `--web.metrics-path` | `/metrics` | Path under which to expose metrics |
| `--web.report-path` | `/report` | Path for the Duplicati webhook (`send-http-url`) |
| `--api.timeout` | `10s` | HTTP timeout for Duplicati API requests |
| `--api.cache-ttl` | `10m` | TTL for cached expensive API responses such as fileset listings (`0` disables caching) |
| `--api.identity-refresh-interval` | `15m` | How often machine identity (`systeminfo`, a large payload) is re-read |
| `--collector.filesets` | `true` | List stored backup versions on every scrape (results cached for `--api.cache-ttl`) |
| `--log.level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--log.format` | `logfmt` | Log format: `logfmt` or `json` |
| `--server` | — | Duplicati endpoint; repeat once per machine (see below) |
| `--version` | — | Print version and exit |

### `--server`

Repeat the flag to scrape multiple Duplicati machines:

```console
$ duplicati-exporter \
    --server 'name=primary;url=https://backup1.example.com:8200;password=secret' \
    --server 'name=secondary;url=https://backup2.example.com:8200;token=abc123'
```

| Key | Required | Description |
|---|---|---|
| `name` | no | Operator-friendly name used in logs only (labels come from Duplicati itself) |
| `url` | **yes** | Base URL of the Duplicati server, e.g. `https://host:8200` |
| `password` | one of | Web-UI/API password (used for `POST /api/v1/auth/login`) |
| `token` | one of | Pre-issued API token (skips login) |
| `insecure-skip-verify` | no | Skip TLS certificate verification (`true`/`false`) |

A bare URL (`--server https://backup1:8200`) is accepted as a shorthand for
`url=…`; you still need to supply `password` or `token`.

Separate keys with `;`. To use a literal `;` (or `\`) inside a value such as a
password, escape it with a backslash: `password=a\;b`. Several servers can also be
supplied as `DUPLICATI_EXPORTER_SERVER_<n>` environment variables.

**Validation and discovery happen at startup.** On boot the exporter validates every
flag, logs in to each endpoint and resolves:

* `machine_name` and the server timezone from `GET /api/v1/systeminfo`
* `machine_id` from the `machine-id` backup option, falling back to the endpoint host
  when no backup declares one

If any endpoint is unreachable, the credentials are wrong or the timezone cannot be
resolved, the exporter exits immediately — it does not start in a degraded state.
The identity is re-read behind a mutex every `--api.identity-refresh-interval`, not on every scrape.

### Environment variables (containers)

Every flag maps to an environment variable with the `DUPLICATI_EXPORTER_` prefix
and `.`/`-` replaced by `_`:

```console
$ DUPLICATI_EXPORTER_WEB_LISTEN_ADDRESS=:9685
$ DUPLICATI_EXPORTER_SERVER='name=primary;url=https://backup1:8200;password=secret'
$ DUPLICATI_EXPORTER_LOG_LEVEL=debug
```

## How to configure Duplicati

The exporter polls the API, so no Duplicati-side report configuration is *required*.
If you also want per-run transfer deltas and `ParsedResult` (webhook metrics), add to
the Duplicati backup's advanced options:

```
--send-http-url=http://<exporter-host>:9685/report
--send-http-result-output-format=Json
--send-http-any-operation=true
```

## Metrics

All time-series are gauges and every timestamp is a **UTC Unix epoch (seconds)**.
Labels use Duplicati's own terminology: `machine_id` and `machine_name` (auto-
discovered from `GET /api/v1/systeminfo`) plus `backup_name` where applicable.

### Per-machine

| Metric | Description |
|---|---|
| `duplicati_machine_scrape_error` | `1` if the API could not be reached for this scrape, `0` otherwise |
| `duplicati_machine_up` | `1` if the API is reachable and authenticated |
| `duplicati_machine_paused` | `1` if the scheduler is paused |
| `duplicati_machine_progress_overall` | Overall progress of the current operation, `0`–`1` |
| `duplicati_machine_notifications_count` | Open notifications, labeled by `backup_name` and `type` (`Information`/`Warning`/`Error`) |
| `duplicati_machine_info` | Constant `1` gauge carrying `timezone`, `version`, `os_type` and `os_version` |
| `duplicati_machine_last_scrape_timestamp_seconds` | UTC timestamp of the last successful scrape of this machine |
| `duplicati_machine_scrape_duration_seconds` | Duration of the last scrape of this machine |

### Per backup

| Metric | Description |
|---|---|
| `duplicati_backup_last_started_time` | Last backup start time (`LastBackupStarted`) |
| `duplicati_backup_last_finished_time` | Last backup finish time (`LastBackupFinished`) |
| `duplicati_backup_last_duration_seconds` | Duration of the last backup run |
| `duplicati_backup_last_date_time` | Newest remote fileset date (`LastBackupDate`) — advances only on a successful upload |
| `duplicati_backup_list_count` | Number of backups on the backend (`BackupListCount`) |
| `duplicati_backup_total_quota_bytes` | Total quota space on the backend |
| `duplicati_backup_free_quota_bytes` | Free quota space on the backend |
| `duplicati_backup_assigned_quota_bytes` | Assigned quota space on the backend |
| `duplicati_backup_target_files_bytes` | Size of files on the backend (`TargetFilesSize`) |
| `duplicati_backup_target_files_count` | Number of files on the backend (`TargetFilesCount`) |
| `duplicati_backup_target_filesets_count` | Number of filesets on the backend |
| `duplicati_backup_source_files_bytes` | Size of source files (`SourceFilesSize`) |
| `duplicati_backup_source_files_count` | Number of source files (`SourceFilesCount`) |
| `duplicati_backup_last_restore_duration_seconds` | Duration of the last restore (when present) |
| `duplicati_backup_last_compact_time` | Last compact start/finish time (when present) |
| `duplicati_backup_last_vacuum_time` | Last vacuum start/finish time (when present) |
| `duplicati_backup_last_sync_time` | Last sync start/finish time (when present) |

### Schedule

| Metric | Description |
|---|---|
| `duplicati_backup_next_run_time` | Next scheduled run time (converted from the server timezone to UTC) |
| `duplicati_backup_schedule_last_run_time` | UTC timestamp when the schedule last triggered a run (`Schedule.LastRun`) |

### Filesets (labeled by `version`)

| Metric | Description |
|---|---|
| `duplicati_backup_filesets_count` | Number of stored versions |
| `duplicati_backup_fileset_time` | Version timestamp |
| `duplicati_backup_fileset_file_count` | Files in the version |
| `duplicati_backup_fileset_file_sizes` | Size of the version |
| `duplicati_backup_fileset_is_full` | `1` if the version is a full backup |

### Derived (computed by the exporter)

| Metric | Description |
|---|---|
| `duplicati_backup_stale` | `1` if the scheduled `NextRun` has passed but no finished/successful backup since |
| `duplicati_backup_failure` | `1` if an active error notification exists for the backup |

`duplicati_backup_stale` is `1` when `now > next_run_time` **and**
(`last_finished_time < next_run_time` **or** `last_date_time < next_run_time`).
A failed run advances `last_finished_time` but not `last_date_time`, so the `OR`
catches both "ran and failed" and "did not run at all". Backups without a schedule
never emit the metric.

`next_run_time` comes from `Schedule.Time`, which Duplicati advances after every
scheduled run, so the alert tracks Duplicati's own scheduling rather than a
re-implementation of its rules.

### Webhook (in-memory, secondary)

| Metric | Description |
|---|---|
| `duplicati_backup_bytes_uploaded_total` | Bytes uploaded in the last run |
| `duplicati_backup_bytes_downloaded_total` | Bytes downloaded in the last run |
| `duplicati_backup_files_uploaded_total` | Files uploaded in the last run |
| `duplicati_backup_files_downloaded_total` | Files downloaded in the last run |
| `duplicati_backup_files_deleted_total` | Files deleted in the last run |
| `duplicati_backup_folders_created_total` | Folders created in the last run |
| `duplicati_backup_retry_attempts` | Retry attempts of the last run |
| `duplicati_backup_last_result` | `0` Success, `1` Warning, `2` Fatal, `3` Unknown |
| `duplicati_backup_last_run_begin_time` | UTC timestamp when the last reported run began |
| `duplicati_backup_last_run_end_time` | UTC timestamp when the last reported run ended |
| `duplicati_backup_last_run_duration_seconds` | Duration of the last reported run |

### Fileset listing

Version metrics require listing the stored versions of a backup, which makes Duplicati
hit the backend. Results are cached for `--api.cache-ttl`; disable the whole family
with `--collector.filesets=false` if your backup runs too frequently or the backend is
slow to enumerate.

### Standard metrics

`go_*`, `process_*` and `duplicati_build_info{version,revision,goversion}` are
exposed as well.

## Error handling

`/metrics` returns **HTTP 500 only if every configured machine failed** for that
scrape — Prometheus then marks the exporter target as down (`up == 0`). If only some
machines are down, the scrape succeeds and the failure is exposed per machine:

```text
duplicati_machine_scrape_error{machine_id="…",machine_name="…"} 1
duplicati_machine_up{machine_id="…",machine_name="…"} 0
```

This lets you alert on individual Duplicati machines without losing visibility into
the healthy ones.

## Example Prometheus configuration

See [`examples/prometheus.yml`](examples/prometheus.yml) for a full scrape config and
[`examples/alerts.yml`](examples/alerts.yml) for alert rules.

```yaml
scrape_configs:
  - job_name: duplicati
    scrape_interval: 60s
    scrape_timeout: 30s
    static_configs:
      - targets: ["exporter:9685"]
```

## Health checks

- `GET /-/healthy` and `GET /healthz` — liveness (returns `200` once the HTTP server
  is up)
- `GET /-/ready` — readiness

## Development

```console
$ make help          # show available targets
$ make coverage      # run tests with coverage
$ make build         # cross-compile into dist/
$ make test-coverage-report  # open coverage report in browser
$ make lint         # go vet
$ make notice       # regenerate NOTICE from the module graph
```

### Licence compliance

The third-party licence inventory lives in [`NOTICE`](NOTICE), generated with
[go-licence-detector](https://github.com/elastic/go-licence-detector):

```console
$ make notice
```

The detector is declared as a Go tool in `go.mod`, so no global install is
needed. Configuration lives in [`.github/`](.github/):

| File | Purpose |
|---|---|
| `licence-rules.json` | Allow-list of licences accepted for dependencies |
| `licence-overrides.json` | Newline-delimited JSON overrides for modules the detector cannot classify |
| `NOTICE.tmpl` | Template used for the generated dependency sections |

[`licence-check.yml`](.github/workflows/licence-check.yml) runs on pull
requests that touch `go.mod`, `go.sum` or `NOTICE`. It fails when a dependency
carries a non-allowed licence, and fails (or auto-commits on Renovate PRs) when
`NOTICE` no longer matches the module graph.

### Project layout

```text
cmd/duplicati-exporter/   entry point
internal/config/          flag parsing and startup validation
internal/duplicati/       Duplicati HTTP API client, discovery and timezone handling
internal/collector/       Prometheus metric definitions
internal/server/          HTTP endpoints (/metrics, /report, health)
internal/webhook/         in-memory store for webhook reports
internal/buildinfo/       version metadata injected via -ldflags
internal/log/             slog helpers
examples/                 Prometheus scrape config, alert rules, demo stack
.github/                  licence rules, NOTICE template and compliance workflow
```

## License

Licensed under the Apache License, Version 2.0 — see [LICENSE](LICENSE) and
[NOTICE](NOTICE).
