// Command worker runs River background jobs (notifications, reminders, maintenance).
//
// T1.1 skeleton: the River client and workers are added in T1.6.
package main

import (
	"log/slog"
	"os"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/buildinfo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	info := buildinfo.Get()

	logger.Info("worker starting",
		slog.String("version", info.Version),
		slog.String("commit", info.Commit),
		slog.String("go", info.GoVersion),
	)
	logger.Warn("river worker not implemented yet (phase 1, task T1.6)")
}
