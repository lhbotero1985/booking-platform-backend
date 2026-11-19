//go:build integration

// Package pgtest starts a disposable PostgreSQL/PostGIS container for integration tests.
// Only compiled with -tags=integration; requires Docker.
package pgtest

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image matches the DEV compose environment.
const Image = "postgis/postgis:17-3.5"

// Container is a running test database.
type Container struct {
	URL string
	c   *tcpostgres.PostgresContainer
}

// Start launches the container and waits until it accepts connections. Credentials are
// synthetic and local to the container. It retries a few times because Docker Desktop on
// Windows intermittently refuses provider creation when several test packages start
// containers concurrently ("failed to create Docker provider").
func Start(ctx context.Context) (*Container, error) {
	const attempts = 4
	var err error
	for i := 1; i <= attempts; i++ {
		var c *Container
		if c, err = start(ctx); err == nil {
			return c, nil
		}
		if i < attempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(i) * 2 * time.Second):
			}
		}
	}
	return nil, fmt.Errorf("pgtest: %d attempts: %w", attempts, err)
}

func start(ctx context.Context) (*Container, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	c, err := tcpostgres.Run(ctx, Image,
		tcpostgres.WithDatabase("booking_test"),
		tcpostgres.WithUsername("booking_test"),
		tcpostgres.WithPassword("booking_test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		if c != nil {
			_ = testcontainers.TerminateContainer(c)
		}
		return nil, fmt.Errorf("pgtest: start %s: %w", Image, err)
	}
	url, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(c)
		return nil, fmt.Errorf("pgtest: connection string: %w", err)
	}
	return &Container{URL: url, c: c}, nil
}

// Terminate stops and removes the container.
func (c *Container) Terminate() {
	if c != nil && c.c != nil {
		_ = testcontainers.TerminateContainer(c.c)
	}
}
