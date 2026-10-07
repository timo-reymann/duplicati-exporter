// Command duplicati-exporter exposes Prometheus metrics for Duplicati backups by
// polling the Duplicati Server HTTP API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/timo-reymann/duplicati-exporter/internal/buildinfo"
	"github.com/timo-reymann/duplicati-exporter/internal/collector"
	"github.com/timo-reymann/duplicati-exporter/internal/config"
	"github.com/timo-reymann/duplicati-exporter/internal/duplicati"
	"github.com/timo-reymann/duplicati-exporter/internal/log"
	"github.com/timo-reymann/duplicati-exporter/internal/server"
	"github.com/timo-reymann/duplicati-exporter/internal/webhook"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cfg, err := config.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	if cfg.ShowVersion {
		info := buildinfo.Get()
		fmt.Printf("duplicati-exporter %s\n", info.Version)
		fmt.Printf("  revision:  %s\n", info.Revision)
		fmt.Printf("  buildtime: %s\n", info.BuildTime)
		fmt.Printf("  go:        %s\n", info.GoVersion)
		return 0
	}

	logger, err := log.New(log.Options{Level: cfg.LogLevel, Format: cfg.LogFormat})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	slog.SetDefault(logger)

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", slog.Any("err", err))
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Startup discovery: fail fast when a configured endpoint cannot be read, so
	// the exporter never runs in a degraded state with partial label sets.
	targets, code := discoverAll(ctx, cfg, logger)
	if code != 0 {
		return code
	}

	info := buildinfo.Get()
	logger.Info("starting duplicati-exporter",
		slog.String("version", info.Version),
		slog.String("revision", info.Revision),
		slog.Int("machines", len(targets)))

	store := webhook.NewStore()
	exporter := collector.New(collector.Options{
		Targets:                 targets,
		Webhooks:                store,
		Timeout:                 cfg.APITimeout,
		FilesetsEnabled:         cfg.FilesetsEnabled,
		FilesetsCacheTTL:        cfg.APICacheTTL,
		IdentityRefreshInterval: cfg.IdentityRefresh,
		Logger:                  logger,
		Build:                   info,
	})

	srv := server.New(server.Config{
		ListenAddress: cfg.WebListenAddress,
		MetricsPath:   cfg.WebMetricsPath,
		ReportPath:    cfg.WebReportPath,
		Logger:        logger,
		Version:       info.Version,
	}, exporter, store)

	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("server stopped", slog.Any("err", err))
		return 1
	}
	logger.Info("shutdown complete")
	return 0
}

// discoverAll resolves the identity of every configured machine. Any failure
// aborts startup.
func discoverAll(ctx context.Context, cfg *config.Config, logger *slog.Logger) ([]*collector.Target, int) {
	targets := make([]*collector.Target, 0, len(cfg.Servers))

	for _, sc := range cfg.Servers {
		client := duplicati.NewClient(sc.URL, sc.Password, sc.Token, sc.InsecureSkipVerify, cfg.APITimeout)

		discoverCtx, cancel := context.WithTimeout(ctx, 3*cfg.APITimeout+5*time.Second)
		info, err := duplicati.Discover(discoverCtx, client)
		cancel()
		if err != nil {
			logger.Error("startup discovery failed",
				slog.String("server", sc.NameOrHost()),
				slog.String("url", sc.URL.String()),
				slog.Any("err", err))
			return nil, 1
		}

		identity, _ := info.Snapshot()
		logger.Info("discovered Duplicati machine",
			slog.String("server", sc.NameOrHost()),
			slog.String("machine_id", identity.MachineID),
			slog.String("machine_name", identity.MachineName),
			slog.String("timezone", identity.Timezone),
			slog.String("version", identity.Version),
			slog.Time("discovered_at", info.DiscoveredAt()))

		targets = append(targets, &collector.Target{
			Name:    sc.NameOrHost(),
			Client:  client,
			Machine: info,
		})
	}

	return targets, 0
}
