// Package buildinfo exposes build metadata injected at link time.
//
// Values are set with:
//
//	go build -ldflags "-X booking-platform-backend/internal/platform/buildinfo.version=v1.2.3 \
//	  -X booking-platform-backend/internal/platform/buildinfo.commit=abc123"
package buildinfo

import "runtime"

// Set via -ldflags; defaults are used for local builds.
var (
	version = "dev"
	commit  = "unknown"
)

// Info describes the running binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"goVersion"`
}

// Get returns the build information of the running binary.
func Get() Info {
	return Info{
		Version:   version,
		Commit:    commit,
		GoVersion: runtime.Version(),
	}
}
