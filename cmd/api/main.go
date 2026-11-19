// Command api runs the booking platform HTTP API.
//
// T1.1 skeleton: the HTTP server, configuration and health checks are added in T1.2–T1.4.
package main

import (
	"log/slog"
	"os"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/buildinfo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	info := buildinfo.Get()

	logger.Info("api starting",
		slog.String("version", info.Version),
		slog.String("commit", info.Commit),
		slog.String("go", info.GoVersion),
	)
	logger.Warn("http server not implemented yet (phase 1, task T1.2)")
}
