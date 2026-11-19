package buildinfo

import (
	"runtime"
	"testing"
)

func TestGetDefaults(t *testing.T) {
	got := Get()

	if got.Version != "dev" {
		t.Errorf("Version = %q, want %q", got.Version, "dev")
	}
	if got.Commit != "unknown" {
		t.Errorf("Commit = %q, want %q", got.Commit, "unknown")
	}
	if got.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", got.GoVersion, runtime.Version())
	}
}

func TestGetReflectsLinkerValues(t *testing.T) {
	origVersion, origCommit := version, commit
	t.Cleanup(func() { version, commit = origVersion, origCommit })

	version, commit = "v1.2.3", "abc123"

	got := Get()
	if got.Version != "v1.2.3" || got.Commit != "abc123" {
		t.Errorf("Get() = %+v, want version v1.2.3 and commit abc123", got)
	}
}
