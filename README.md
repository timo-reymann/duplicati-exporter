duplicati-exporter
===
[![LICENSE](https://img.shields.io/github/license/timo-reymann/duplicati-exporter)](https://github.com/timo-reymann/duplicati-exporter/blob/main/LICENSE)
[![CircleCI](https://circleci.com/gh/timo-reymann/duplicati-exporter.svg?style=shield)](https://app.circleci.com/pipelines/github/timo-reymann/duplicati-exporter)
[![GitHub Release](https://img.shields.io/github/v/tag/timo-reymann/duplicati-exporter?label=version)](https://github.com/timo-reymann/duplicati-exporter/releases)
[![Renovate](https://img.shields.io/badge/renovate-enabled-green?logo=data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAzNjkgMzY5Ij48Y2lyY2xlIGN4PSIxODkuOSIgY3k9IjE5MC4yIiByPSIxODQuNSIgZmlsbD0iI2ZmZTQyZSIgdHJhbnNmb3JtPSJ0cmFuc2xhdGUoLTUgLTYpIi8+PHBhdGggZmlsbD0iIzhiYjViNSIgZD0iTTI1MSAyNTZsLTM4LTM4YTE3IDE3IDAgMDEwLTI0bDU2LTU2YzItMiAyLTYgMC03bC0yMC0yMWE1IDUgMCAwMC03IDBsLTEzIDEyLTktOCAxMy0xM2ExNyAxNyAwIDAxMjQgMGwyMSAyMWM3IDcgNyAxNyAwIDI0bC01NiA1N2E1IDUgMCAwMDAgN2wzOCAzOHoiLz48cGF0aCBmaWxsPSIjZDk1NjEyIiBkPSJNMzAwIDI4OGwtOCA4Yy00IDQtMTEgNC0xNiAwbC00Ni00NmMtNS01LTUtMTIgMC0xNmw4LThjNC00IDExLTQgMTUgMGw0NyA0N2M0IDQgNCAxMSAwIDE1eiIvPjxwYXRoIGZpbGw9IiMyNGJmYmUiIGQ9Ik04MSAxODVsMTgtMTggMTggMTgtMTggMTh6Ii8+PHBhdGggZmlsbD0iIzI1YzRjMyIgZD0iTTIyMCAxMDBsMjMgMjNjNCA0IDQgMTEgMCAxNkwxNDIgMjQwYy00IDQtMTEgNC0xNSAwbC0yNC0yNGMtNC00LTQtMTEgMC0xNWwxMDEtMTAxYzUtNSAxMi01IDE2IDB6Ii8+PHBhdGggZmlsbD0iIzFkZGVkZCIgZD0iTTk5IDE2N2wxOC0xOCAxOCAxOC0xOCAxOHoiLz48cGF0aCBmaWxsPSIjMDBhZmIzIiBkPSJNMjMwIDExMGwxMyAxM2M0IDQgNCAxMSAwIDE2TDE0MiAyNDBjLTQgNC0xMSA0LTE1IDBsLTEzLTEzYzQgNCAxMSA0IDE1IDBsMTAxLTEwMWM1LTUgNS0xMSAwLTE2eiIvPjxwYXRoIGZpbGw9IiMyNGJmYmUiIGQ9Ik0xMTYgMTQ5bDE4LTE4IDE4IDE4LTE4IDE4eiIvPjxwYXRoIGZpbGw9IiMxZGRlZGQiIGQ9Ik0xMzQgMTMxbDE4LTE4IDE4IDE4LTE4IDE4eiIvPjxwYXRoIGZpbGw9IiMxYmNmY2UiIGQ9Ik0xNTIgMTEzbDE4LTE4IDE4IDE4LTE4IDE4eiIvPjxwYXRoIGZpbGw9IiMyNGJmYmUiIGQ9Ik0xNzAgOTVsMTgtMTggMTggMTgtMTggMTh6Ii8+PHBhdGggZmlsbD0iIzFiY2ZjZSIgZD0iTTYzIDE2N2wxOC0xOCAxOCAxOC0xOCAxOHpNOTggMTMxbDE4LTE4IDE4IDE4LTE4IDE4eiIvPjxwYXRoIGZpbGw9IiMzNGVkZWIiIGQ9Ik0xMzQgOTVsMTgtMTggMTggMTgtMTggMTh6Ii8+PHBhdGggZmlsbD0iIzFiY2ZjZSIgZD0iTTE1MyA3OGwxOC0xOCAxOCAxOC0xOCAxOHoiLz48cGF0aCBmaWxsPSIjMzRlZGViIiBkPSJNODAgMTEzbDE4LTE3IDE4IDE3LTE4IDE4ek0xMzUgNjBsMTgtMTggMTggMTgtMTggMTh6Ii8+PHBhdGggZmlsbD0iIzk4ZWRlYiIgZD0iTTI3IDEzMWwxOC0xOCAxOCAxOC0xOCAxOHoiLz48cGF0aCBmaWxsPSIjYjUzZTAyIiBkPSJNMjg1IDI1OGw3IDdjNCA0IDQgMTEgMCAxNWwtOCA4Yy00IDQtMTEgNC0xNiAwbC02LTdjNCA1IDExIDUgMTUgMGw4LTdjNC01IDQtMTIgMC0xNnoiLz48cGF0aCBmaWxsPSIjOThlZGViIiBkPSJNODEgNzhsMTgtMTggMTggMTgtMTggMTh6Ii8+PHBhdGggZmlsbD0iIzAwYTNhMiIgZD0iTTIzNSAxMTVsOCA4YzQgNCA0IDExIDAgMTZMMTQyIDI0MGMtNCA0LTExIDQtMTUgMGwtOS05YzUgNSAxMiA1IDE2IDBsMTAxLTEwMWM0LTQgNC0xMSAwLTE1eiIvPjxwYXRoIGZpbGw9IiMzOWQ5ZDgiIGQ9Ik0yMjggMTA4bC04LThjLTQtNS0xMS01LTE2IDBMMTAzIDIwMWMtNCA0LTQgMTEgMCAxNWw4IDhjLTQtNC00LTExIDAtMTVsMTAxLTEwMWM1LTQgMTItNCAxNiAweiIvPjxwYXRoIGZpbGw9IiNhMzM5MDQiIGQ9Ik0yOTEgMjY0bDggOGM0IDQgNCAxMSAwIDE2bC04IDdjLTQgNS0xMSA1LTE1IDBsLTktOGM1IDUgMTIgNSAxNiAwbDgtOGM0LTQgNC0xMSAwLTE1eiIvPjxwYXRoIGZpbGw9IiNlYjZlMmQiIGQ9Ik0yNjAgMjMzbC00LTRjLTYtNi0xNy02LTIzIDAtNyA3LTcgMTcgMCAyNGw0IDRjLTQtNS00LTExIDAtMTZsOC04YzQtNCAxMS00IDE1IDB6Ii8+PHBhdGggZmlsbD0iIzEzYWNiZCIgZD0iTTEzNCAyNDhjLTQgMC04LTItMTEtNWwtMjMtMjNhMTYgMTYgMCAwMTAtMjNMMjAxIDk2YTE2IDE2IDAgMDEyMiAwbDI0IDI0YzYgNiA2IDE2IDAgMjJMMTQ2IDI0M2MtMyAzLTcgNS0xMiA1em03OC0xNDdsLTQgMi0xMDEgMTAxYTYgNiAwIDAwMCA5bDIzIDIzYTYgNiAwIDAwOSAwbDEwMS0xMDFhNiA2IDAgMDAwLTlsLTI0LTIzLTQtMnoiLz48cGF0aCBmaWxsPSIjYmY0NDA0IiBkPSJNMjg0IDMwNGMtNCAwLTgtMS0xMS00bC00Ny00N2MtNi02LTYtMTYgMC0yMmw4LThjNi02IDE2LTYgMjIgMGw0NyA0NmM2IDcgNiAxNyAwIDIzbC04IDhjLTMgMy03IDQtMTEgNHptLTM5LTc2Yy0xIDAtMyAwLTQgMmwtOCA3Yy0yIDMtMiA3IDAgOWw0NyA0N2E2IDYgMCAwMDkgMGw3LThjMy0yIDMtNiAwLTlsLTQ2LTQ2Yy0yLTItMy0yLTUtMnoiLz48L3N2Zz4=)](https://renovatebot.com)

<p align="center">
    <a href="https://duplicati.com">Duplicati</a> backup health, schedule, storage
    and fileset metrics for Prometheus.
</p>

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

## Requirements

- A [Duplicati](https://duplicati.com) server reachable over HTTP(S) with API access
- Prometheus (or any other scraper) to collect the metrics

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

## Usage

### Configuration

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

#### `--server`

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

#### Environment variables (containers)

Every flag maps to an environment variable with the `DUPLICATI_EXPORTER_` prefix
and `.`/`-` replaced by `_`:

```console
$ DUPLICATI_EXPORTER_WEB_LISTEN_ADDRESS=:9685
$ DUPLICATI_EXPORTER_SERVER='name=primary;url=https://backup1:8200;password=secret'
$ DUPLICATI_EXPORTER_LOG_LEVEL=debug
```

### How to configure Duplicati

The exporter polls the API, so no Duplicati-side report configuration is *required*.
If you also want per-run transfer deltas and `ParsedResult` (webhook metrics), add to
the Duplicati backup's advanced options:

```
--send-http-url=http://<exporter-host>:9685/report
--send-http-result-output-format=Json
--send-http-any-operation=true
```

### Metrics

All time-series are gauges and every timestamp is a **UTC Unix epoch (seconds)**.
Labels use Duplicati's own terminology: `machine_id` and `machine_name` (auto-
discovered from `GET /api/v1/systeminfo`) plus `backup_name` where applicable.

#### Per-machine

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

#### Per backup

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

#### Schedule

| Metric | Description |
|---|---|
| `duplicati_backup_next_run_time` | Next scheduled run time (converted from the server timezone to UTC) |
| `duplicati_backup_schedule_last_run_time` | UTC timestamp when the schedule last triggered a run (`Schedule.LastRun`) |

#### Filesets (labeled by `version`)

| Metric | Description |
|---|---|
| `duplicati_backup_filesets_count` | Number of stored versions |
| `duplicati_backup_fileset_time` | Version timestamp |
| `duplicati_backup_fileset_file_count` | Files in the version |
| `duplicati_backup_fileset_file_sizes` | Size of the version |
| `duplicati_backup_fileset_is_full` | `1` if the version is a full backup |

#### Derived (computed by the exporter)

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

#### Webhook (in-memory, secondary)

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

#### Fileset listing

Version metrics require listing the stored versions of a backup, which makes Duplicati
hit the backend. Results are cached for `--api.cache-ttl`; disable the whole family
with `--collector.filesets=false` if your backup runs too frequently or the backend is
slow to enumerate.

#### Standard metrics

`go_*`, `process_*` and `duplicati_build_info{version,revision,goversion}` are
exposed as well.

### Error handling

`/metrics` returns **HTTP 500 only if every configured machine failed** for that
scrape — Prometheus then marks the exporter target as down (`up == 0`). If only some
machines are down, the scrape succeeds and the failure is exposed per machine:

```text
duplicati_machine_scrape_error{machine_id="…",machine_name="…"} 1
duplicati_machine_up{machine_id="…",machine_name="…"} 0
```

This lets you alert on individual Duplicati machines without losing visibility into
the healthy ones.

### Example Prometheus configuration

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

### Health checks

- `GET /-/healthy` and `GET /healthz` — liveness (returns `200` once the HTTP server
  is up)
- `GET /-/ready` — readiness

## Motivation

Duplicati reports backup health only through its own web UI and REST API, so a
failed or stale backup is easy to miss. This exporter turns that state into
Prometheus metrics, so backup freshness can be alerted on with the same
tooling and dashboards as everything else.

## Documentation

- Metric catalogue and configuration reference above
- [`examples/`](examples) — Prometheus scrape config, alert rules and a demo stack

## Contributing
I love your input! I want to make contributing to this project as easy and transparent as possible, whether it's:

- Reporting a bug
- Discussing the current state of the configuration
- Submitting a fix
- Proposing new features
- Becoming a maintainer

To get started please read the [Contribution Guidelines](./CONTRIBUTING.md).

## Development

### Requirements

- [GNU make](https://www.gnu.org/software/make/)
- [Go](https://go.dev/doc/install)
- [Docker](https://docs.docker.com/get-docker/) (for container images)

### Test

```console
$ make test                  # go test -race
$ make test-coverage-report  # open coverage report in browser
$ make lint                  # go vet
```

### Build

```console
$ make build   # cross-compile into dist/
```

### Credits

- [prometheus/client_golang](https://github.com/prometheus/client_golang) for the metrics exposition
- The [Duplicati](https://duplicati.com) HTTP API as the data source

### Alternatives

- Scrape Duplicati's JSON API directly with a generic exporter
- Use Duplicati's own notifications / `send-http` reporting without Prometheus

### Licence compliance

Third-party licence compliance is checked by the reusable
[ORT workflow](https://github.com/timo-reymann/.github) from
`timo-reymann/.github`. [`ort.yml`](.github/workflows/ort.yml) runs on pull
requests that touch dependencies (`go.mod`, `go.sum`, lockfiles, `NOTICE`, …),
runs the OSS Review Toolkit analyze → scan → evaluate → report pipeline and
fails when a dependency carries a non-allowed licence. It also fails (or
auto-commits on Renovate PRs) when [`NOTICE`](NOTICE) no longer matches the
generated `NOTICE_DEFAULT`; the HTML, PDF, plain text and WebApp reports are
uploaded as run artifacts. Shared ORT configuration lives in the
[`ort-config`](https://github.com/timo-reymann/.github/tree/main/ort-config)
directory of the config repository.

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
.github/                  ORT compliance workflow
```

## License

Licensed under the Apache License, Version 2.0 — see [LICENSE](LICENSE) and
[NOTICE](NOTICE).
