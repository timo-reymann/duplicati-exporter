// Package buildinfo exposes version metadata that is injected at build time
// via -ldflags "-X github.com/timo-reymann/duplicati-exporter/internal/buildinfo.Version=…".
package buildinfo

import (
	"fmt"
	"runtime"
)

// These variables are set at build time via -ldflags.
var (
	// Version is the semantic version of the build (e.g. "v1.2.3" or "dev").
	Version = "dev"
	// GitSha is the short git revision the binary was built from.
	GitSha = "unknown"
	// BuildTime is the UTC build timestamp in RFC3339 format.
	BuildTime = "unknown"
)

// Info describes the current build.
type Info struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// Get returns the build information.
func Get() Info {
	return Info{
		Version:   Version,
		Revision:  GitSha,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}

// String renders the build information as a single line.
func (i Info) String() string {
	return fmt.Sprintf("version=%s revision=%s buildtime=%s go=%s", i.Version, i.Revision, i.BuildTime, i.GoVersion)
}
