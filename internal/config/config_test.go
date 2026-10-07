package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse([]string{"--server", "url=https://duplicati.example.com:8200;password=secret"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.WebListenAddress != ":9685" {
		t.Errorf("WebListenAddress = %q, want :9685", cfg.WebListenAddress)
	}
	if cfg.WebMetricsPath != "/metrics" {
		t.Errorf("WebMetricsPath = %q, want /metrics", cfg.WebMetricsPath)
	}
	if cfg.WebReportPath != "/report" {
		t.Errorf("WebReportPath = %q, want /report", cfg.WebReportPath)
	}
	if cfg.APITimeout != 10*time.Second {
		t.Errorf("APITimeout = %v, want 10s", cfg.APITimeout)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestParseMultipleServers(t *testing.T) {
	cfg, err := Parse([]string{
		"--server", "name=primary;url=https://one.example.com:8200;password=a",
		"--server", "name=secondary;url=https://two.example.com:8200;token=b",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("len(Servers) = %d, want 2", len(cfg.Servers))
	}
	if cfg.Servers[0].Name != "primary" || cfg.Servers[1].Name != "secondary" {
		t.Errorf("server names = %q, %q", cfg.Servers[0].Name, cfg.Servers[1].Name)
	}
	if cfg.Servers[0].Password != "a" {
		t.Errorf("Password = %q, want a", cfg.Servers[0].Password)
	}
	if cfg.Servers[1].Token != "b" {
		t.Errorf("Token = %q, want b", cfg.Servers[1].Token)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestParseBareURL(t *testing.T) {
	cfg, err := Parse([]string{"--server", "duplicati.example.com:8200"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("len(Servers) = %d, want 1", len(cfg.Servers))
	}
	if got := cfg.Servers[0].URL.Scheme; got != "https" {
		t.Errorf("scheme = %q, want https (defaulted)", got)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() error = nil, want error: missing password/token")
	}
}

func TestParseInsecureSkipVerify(t *testing.T) {
	cfg, err := Parse([]string{
		"--server", "url=https://self-signed.example.com;password=x;insecure-skip-verify=true",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !cfg.Servers[0].InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty server", []string{"--server", ""}, "empty value"},
		{"missing url", []string{"--server", "password=x"}, "missing url"},
		{"unknown key", []string{"--server", "url=https://x;foo=bar"}, "unknown key"},
		{"bad scheme", []string{"--server", "url=ftp://x;password=y"}, "scheme must be"},
		{"bad bool", []string{"--server", "url=https://x;password=y;insecure-skip-verify=maybe"}, "invalid insecure-skip-verify"},
		{"malformed segment", []string{"--server", "url=https://x;novalue"}, "malformed segment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args)
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse() error = %v, want contains %q", err, tt.want)
			}
		})
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"no servers", func(c *Config) { c.Servers = nil }, "at least one --server"},
		{"missing auth", func(c *Config) { c.Servers[0].Password = ""; c.Servers[0].Token = "" }, "password or token"},
		{"both auth", func(c *Config) { c.Servers[0].Token = "t" }, "mutually exclusive"},
		{"duplicate url", func(c *Config) {
			c.Servers = append(c.Servers, ServerConfig{URL: c.Servers[0].URL, Password: "y"})
		}, "duplicate url"},
		{"bad metrics path", func(c *Config) { c.WebMetricsPath = "metrics" }, "must start with"},
		{"same paths", func(c *Config) { c.WebReportPath = c.WebMetricsPath }, "must differ"},
		{"bad timeout", func(c *Config) { c.APITimeout = 0 }, "greater than zero"},
		{"negative ttl", func(c *Config) { c.APICacheTTL = -1 }, "must not be negative"},
		{"bad log level", func(c *Config) { c.LogLevel = "verbose" }, "--log.level"},
		{"bad log format", func(c *Config) { c.LogFormat = "xml" }, "--log.format"},
		{"metrics path root", func(c *Config) { c.WebMetricsPath = "/" }, "used by the exporter"},
		{"report path healthz", func(c *Config) { c.WebReportPath = "/healthz" }, "used by the exporter"},
		{"empty listen", func(c *Config) { c.WebListenAddress = "" }, "--web.listen-address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]string{"--server", "url=https://d.example.com;password=x"})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			tt.mutate(cfg)
			err = cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() error = %v, want contains %q", err, tt.want)
			}
		})
	}
}

func TestNameOrHost(t *testing.T) {
	cfg, err := Parse([]string{"--server", "url=https://d.example.com:8200;password=x"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := cfg.Servers[0].NameOrHost(); got != "d.example.com:8200" {
		t.Errorf("NameOrHost() = %q, want d.example.com:8200", got)
	}
	cfg.Servers[0].Name = "primary"
	if got := cfg.Servers[0].NameOrHost(); got != "primary" {
		t.Errorf("NameOrHost() = %q, want primary", got)
	}
}

func TestParseEscapedSemicolonInPassword(t *testing.T) {
	cfg, err := Parse([]string{"--server", `url=https://x.example.com;password=a\;b=c\\d;name=n`})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := cfg.Servers[0].Password, `a;b=c\d`; got != want {
		t.Errorf("Password = %q, want %q", got, want)
	}
	if cfg.Servers[0].Name != "n" {
		t.Errorf("Name = %q, want n", cfg.Servers[0].Name)
	}
}

func TestParseInvalidEnvValues(t *testing.T) {
	t.Setenv(EnvPrefix+"API_TIMEOUT", "10")
	t.Setenv(EnvPrefix+"COLLECTOR_FILESETS", "maybe")
	_, err := Parse([]string{"--server", "https://x.example.com"})
	if err == nil {
		t.Fatal("Parse() error = nil, want error")
	}
	for _, want := range []string{"API_TIMEOUT", "COLLECTOR_FILESETS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Parse() error = %v, want contains %q", err, want)
		}
	}
}

func TestParseNumberedServerEnvAllowsGaps(t *testing.T) {
	t.Setenv(EnvPrefix+"SERVER_2", "https://two.example.com")
	t.Setenv(EnvPrefix+"SERVER_1", "https://one.example.com")
	t.Setenv(EnvPrefix+"SERVER_10", "https://ten.example.com")
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	var hosts []string
	for _, s := range cfg.Servers {
		hosts = append(hosts, s.URL.Host)
	}
	if got, want := strings.Join(hosts, ","), "one.example.com,two.example.com,ten.example.com"; got != want {
		t.Errorf("servers = %s, want %s", got, want)
	}
}
