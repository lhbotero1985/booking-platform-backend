package app

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/argon2"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/mail"
)

// NewIdentityService wires the identity use cases with their production adapters. inserter
// enqueues jobs in the same transaction as the data (River client).
func NewIdentityService(cfg config.Config, pool *pgxpool.Pool, inserter postgres.JobInserter, logger *slog.Logger, throttle application.AttemptThrottle) (*application.Service, error) {
	hasher, err := argon2.New(argon2.Params{
		MemoryKiB:   cfg.Argon2.MemoryKiB,
		Iterations:  cfg.Argon2.Iterations,
		Parallelism: cfg.Argon2.Parallelism,
	}, cfg.Argon2.MaxConcurrent)
	if err != nil {
		return nil, err
	}
	mailer, err := mail.NewSMTP(mail.Config{
		Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, From: cfg.SMTP.From,
		Username: cfg.SMTP.Username, Password: cfg.SMTP.Password, TLSMode: cfg.SMTP.TLSMode,
	})
	if err != nil {
		return nil, err
	}
	return application.NewService(application.Deps{
		Store:    postgres.NewStore(pool, inserter),
		Hasher:   hasher,
		Mailer:   mailer,
		Clock:    clock.System{},
		Config:   application.Config{PublicAppURL: cfg.PublicAppURL, OwnedBusinessLimit: cfg.OwnedBusinessLimit},
		Logger:   logger,
		Throttle: throttle,
	}), nil
}
