package security_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/argon2"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// TestLoginTimingDoesNotRevealAccounts measures login with an unknown email and with a wrong
// password for an existing account using the real Argon2id hasher. Both must take comparable
// time, otherwise response timing would reveal which emails are registered.
func TestLoginTimingDoesNotRevealAccounts(t *testing.T) {
	hasher, err := argon2.New(argon2.Params{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1}, 2)
	if err != nil {
		t.Fatal(err)
	}
	store, mailer := apptest.NewStore(), &apptest.Mailer{}
	svc := application.NewService(application.Deps{
		Store: store, Hasher: hasher, Mailer: mailer, Clock: clock.NewFixed(time.Now()),
		Random: &apptest.FixedRandom{}, Config: application.Config{PublicAppURL: "https://app.example.test"},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx := context.Background()
	const password = "una clave suficientemente larga"
	if err := svc.Register(ctx, application.RegisterInput{Email: "ana@example.com", Password: password, FullName: "Ana", Phone: "3001234567"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SendVerificationEmail(ctx, store.Enqueued[0]); err != nil {
		t.Fatal(err)
	}
	raw, _ := url.QueryUnescape(regexp.MustCompile(`token=([^\s]+)`).FindStringSubmatch(mailer.Last().Body)[1])
	if err := svc.VerifyEmail(ctx, raw); err != nil {
		t.Fatal(err)
	}

	measure := func(email, pw string) time.Duration {
		start := time.Now()
		_, err := svc.Login(ctx, application.LoginInput{Email: email, Password: pw, ClientType: "WEB"})
		d := time.Since(start)
		if !errors.Is(err, application.ErrInvalidCredentials) {
			t.Fatalf("login(%s) err = %v", email, err)
		}
		return d
	}
	// Samples are interleaved so background load (e.g. other test packages running in
	// parallel) affects both series equally; medians discard outliers.
	const n = 15
	var unknownS, wrongS []time.Duration
	for range n {
		unknownS = append(unknownS, measure("nobody@example.com", password))
		wrongS = append(wrongS, measure("ana@example.com", password+"x"))
	}
	slices.Sort(unknownS)
	slices.Sort(wrongS)
	unknown, wrong := unknownS[n/2], wrongS[n/2]
	ratio := float64(unknown) / float64(wrong)
	t.Logf("unknown email %v, wrong password %v, ratio %.2f", unknown, wrong, ratio)
	if ratio < 0.6 || ratio > 1.6 {
		t.Fatalf("login timing reveals accounts: unknown %v vs wrong password %v (ratio %.2f)", unknown, wrong, ratio)
	}
}
