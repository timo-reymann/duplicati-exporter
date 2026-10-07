// Package config parses and validates the exporter's command line flags.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Environment prefix for flag defaults.
const EnvPrefix = "DUPLICATI_EXPORTER_"

// ServerConfig describes one Duplicati endpoint to poll.
type ServerConfig struct {
	// Name is an operator-friendly identifier used in logs only.
	// Prometheus labels are discovered from Duplicati itself.
	Name string
	// URL is the base URL of the Duplicati server.
	URL *url.URL
	// Password is the Duplicati web UI/API password (mutually exclusive with Token).
	Password string
	// Token is a pre-issued API token (mutually exclusive with Password).
	Token string
	// InsecureSkipVerify disables TLS certificate verification.
	InsecureSkipVerify bool
}

// Config holds the fully parsed exporter configuration.
type Config struct {
	WebListenAddress string
	WebMetricsPath   string
	WebReportPath    string
	APITimeout       time.Duration
	APICacheTTL      time.Duration
	IdentityRefresh  time.Duration
	FilesetsEnabled  bool
	LogLevel         string
	LogFormat        string
	Servers          []ServerConfig
	ShowVersion      bool
}

// stringSlice collects repeated flag values.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// Parse parses args (usually os.Args[1:]) into a Config. Flags fall back to
// DUPLICATI_EXPORTER_* environment variables when not set on the command line.
func Parse(args []string) (*Config, error) {
	cfg := &Config{}
	var envErrs []error
	env := envReader{errs: &envErrs}

	fs := flag.NewFlagSet("duplicati-exporter", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&cfg.WebListenAddress, "web.listen-address", envString("WEB_LISTEN_ADDRESS", ":9685"),
		"Address to listen on for HTTP requests")
	fs.StringVar(&cfg.WebMetricsPath, "web.metrics-path", envString("WEB_METRICS_PATH", "/metrics"),
		"Path under which to expose Prometheus metrics")
	fs.StringVar(&cfg.WebReportPath, "web.report-path", envString("WEB_REPORT_PATH", "/report"),
		"Path Duplicati posts webhook reports to")
	fs.DurationVar(&cfg.APITimeout, "api.timeout", env.duration("API_TIMEOUT", 10*time.Second),
		"HTTP timeout for Duplicati API requests")
	fs.DurationVar(&cfg.APICacheTTL, "api.cache-ttl", env.duration("API_CACHE_TTL", 10*time.Minute),
		"TTL for cached expensive API responses such as fileset listings (0 disables caching)")
	fs.DurationVar(&cfg.IdentityRefresh, "api.identity-refresh-interval", env.duration("API_IDENTITY_REFRESH_INTERVAL", 15*time.Minute),
		"How often to re-read machine identity (systeminfo) from Duplicati")
	fs.BoolVar(&cfg.FilesetsEnabled, "collector.filesets", env.boolean("COLLECTOR_FILESETS", true),
		"List stored backup versions on every scrape (cached for --api.cache-ttl)")
	fs.StringVar(&cfg.LogLevel, "log.level", envString("LOG_LEVEL", "info"),
		"Log level: debug, info, warn or error")
	fs.StringVar(&cfg.LogFormat, "log.format", envString("LOG_FORMAT", "logfmt"),
		"Log format: logfmt or json")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "Print version information and exit")

	var servers stringSlice
	fs.Var(&servers, "server",
		"Duplicati endpoint as 'name=…;url=https://host:8200;password=…' (repeatable)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if len(envErrs) > 0 {
		return nil, errors.Join(envErrs...)
	}

	// Allow the flag or the env var; the env var holds a single server definition
	// (use DUPLICATI_EXPORTER_SERVER_<n> for several).
	if len(servers) == 0 {
		if v := os.Getenv(EnvPrefix + "SERVER"); strings.TrimSpace(v) != "" {
			servers = []string{v}
		}
	}

	// Also accept DUPLICATI_EXPORTER_SERVER_<n> entries, in numeric order. Gaps
	// in the numbering are fine.
	servers = append(servers, numberedServers()...)

	for _, raw := range servers {
		sc, err := parseServer(raw)
		if err != nil {
			return nil, err
		}
		cfg.Servers = append(cfg.Servers, *sc)
	}

	return cfg, nil
}

// splitEscaped splits s on sep. A backslash escapes the next character, so
// values (passwords in particular) can contain the separator as `\;` and a
// literal backslash as `\\`.
func splitEscaped(s string, sep rune) []string {
	var parts []string
	var cur strings.Builder
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == sep:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if escaped {
		cur.WriteRune('\\')
	}
	return append(parts, cur.String())
}

// numberedServers returns the values of DUPLICATI_EXPORTER_SERVER_<n> sorted by n.
func numberedServers() []string {
	prefix := EnvPrefix + "SERVER_"
	type entry struct {
		n int
		v string
	}
	var found []entry
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		suffix, ok := strings.CutPrefix(k, prefix)
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 0 {
			continue
		}
		found = append(found, entry{n, v})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].n < found[j].n })
	out := make([]string, len(found))
	for i, e := range found {
		out[i] = e.v
	}
	return out
}

// parseServer parses "name=primary;url=https://…;password=…". A bare URL is also
// accepted for convenience. Use `\;` for a literal semicolon in a value.
func parseServer(raw string) (*ServerConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("--server: empty value")
	}

	sc := &ServerConfig{}

	// Bare URL shorthand.
	if !strings.Contains(raw, "=") {
		u, err := parseServerURL(raw)
		if err != nil {
			return nil, err
		}
		sc.URL = u
		return sc, nil
	}

	for _, part := range splitEscaped(raw, ';') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("--server: malformed segment %q (want key=value)", part)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch key {
		case "name":
			sc.Name = value
		case "url", "endpoint":
			u, err := parseServerURL(value)
			if err != nil {
				return nil, err
			}
			sc.URL = u
		case "password", "pass":
			sc.Password = value
		case "token":
			sc.Token = value
		case "insecure-skip-verify", "tls-insecure", "skip-verify":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("--server: invalid insecure-skip-verify value %q", value)
			}
			sc.InsecureSkipVerify = b
		default:
			return nil, fmt.Errorf("--server: unknown key %q", key)
		}
	}

	if sc.URL == nil {
		return nil, errors.New("--server: missing url")
	}
	return sc, nil
}

func parseServerURL(value string) (*url.URL, error) {
	if value == "" {
		return nil, errors.New("--server: url must not be empty")
	}
	// Duplicati often configured without scheme; default to https.
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("--server: invalid url %q: %w", value, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("--server: url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("--server: url %q has no host", value)
	}
	return u, nil
}

// Validate checks the configuration for consistency. It is called at startup so
// that misconfiguration prevents the exporter from starting instead of failing
// on the first scrape.
func (c *Config) Validate() error {
	var errs []error

	if strings.TrimSpace(c.WebListenAddress) == "" {
		errs = append(errs, errors.New("--web.listen-address must not be empty"))
	}
	for name, p := range map[string]string{
		"--web.metrics-path": c.WebMetricsPath,
		"--web.report-path":  c.WebReportPath,
	} {
		if !strings.HasPrefix(p, "/") {
			errs = append(errs, fmt.Errorf("%s must start with '/', got %q", name, p))
		}
	}
	for name, p := range map[string]string{
		"--web.metrics-path": c.WebMetricsPath,
		"--web.report-path":  c.WebReportPath,
	} {
		for _, reserved := range reservedPaths {
			if p == reserved {
				errs = append(errs, fmt.Errorf("%s must not be %q, it is used by the exporter itself", name, p))
			}
		}
	}
	if c.WebMetricsPath == c.WebReportPath {
		errs = append(errs, errors.New("--web.metrics-path and --web.report-path must differ"))
	}
	if c.APITimeout <= 0 {
		errs = append(errs, errors.New("--api.timeout must be greater than zero"))
	}
	if c.IdentityRefresh <= 0 {
		errs = append(errs, errors.New("--api.identity-refresh-interval must be greater than zero"))
	}
	if c.APICacheTTL < 0 {
		errs = append(errs, errors.New("--api.cache-ttl must not be negative"))
	}

	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "warning", "error", "":
	default:
		errs = append(errs, fmt.Errorf("--log.level must be debug, info, warn or error, got %q", c.LogLevel))
	}
	switch strings.ToLower(c.LogFormat) {
	case "", "logfmt", "text", "json":
	default:
		errs = append(errs, fmt.Errorf("--log.format must be logfmt or json, got %q", c.LogFormat))
	}

	if len(c.Servers) == 0 {
		errs = append(errs, errors.New("at least one --server is required"))
	}

	seen := make(map[string]struct{}, len(c.Servers))
	for i, s := range c.Servers {
		prefix := fmt.Sprintf("--server #%d", i+1)
		if s.URL == nil {
			errs = append(errs, fmt.Errorf("%s: url is required", prefix))
		} else {
			key := strings.TrimSuffix(s.URL.String(), "/")
			if _, dup := seen[key]; dup {
				errs = append(errs, fmt.Errorf("%s: duplicate url %q", prefix, s.URL.String()))
			}
			seen[key] = struct{}{}
		}
		if s.Password == "" && s.Token == "" {
			errs = append(errs, fmt.Errorf("%s: either password or token is required", prefix))
		}
		if s.Password != "" && s.Token != "" {
			errs = append(errs, fmt.Errorf("%s: password and token are mutually exclusive", prefix))
		}
	}

	return errors.Join(errs...)
}

// reservedPaths are served by the exporter and cannot be reused for the metrics
// or report endpoints (registering them twice would panic the HTTP mux).
var reservedPaths = []string{"/", "/-/healthy", "/-/ready", "/healthz"}

// NameOrHost returns a stable human-readable name for a server config.
func (s ServerConfig) NameOrHost() string {
	if s.Name != "" {
		return s.Name
	}
	if s.URL != nil {
		return s.URL.Host
	}
	return "unknown"
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(EnvPrefix + key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

// envReader reads typed flag defaults from the environment and records
// unparsable values instead of silently falling back to the default.
type envReader struct{ errs *[]error }

func (r envReader) boolean(key string, def bool) bool {
	v, ok := os.LookupEnv(EnvPrefix + key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		*r.errs = append(*r.errs, fmt.Errorf("%s%s: invalid boolean %q", EnvPrefix, key, v))
		return def
	}
	return b
}

func (r envReader) duration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(EnvPrefix + key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		*r.errs = append(*r.errs, fmt.Errorf("%s%s: invalid duration %q (use a unit, e.g. 10s)", EnvPrefix, key, v))
		return def
	}
	return d
}
