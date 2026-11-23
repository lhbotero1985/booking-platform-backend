package argon2

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

var _ application.PasswordHasher = (*Hasher)(nil)

// testParams keep unit tests fast while staying inside the accepted limits.
var testParams = Params{MemoryKiB: minMemoryKiB, Iterations: minIter, Parallelism: 1}

func newHasher(t testing.TB, p Params) *Hasher {
	t.Helper()
	h, err := New(p, 4)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func pw(t testing.TB, s string) domain.Password {
	t.Helper()
	p, err := domain.PasswordForVerification(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var phcPattern = regexp.MustCompile(`^\$argon2id\$v=19\$m=\d+,t=\d+,p=\d+\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)

func TestHashAndVerify(t *testing.T) {
	ctx := context.Background()
	h := newHasher(t, testParams)

	hash, err := h.Hash(ctx, pw(t, "correct horse battery"))
	if err != nil {
		t.Fatal(err)
	}
	if !phcPattern.MatchString(string(hash)) {
		t.Fatalf("hash not in PHC format: %s", hash)
	}
	if strings.Contains(string(hash), "correct horse") {
		t.Fatal("hash contains the password")
	}

	ok, rehash, err := h.Verify(ctx, pw(t, "correct horse battery"), hash)
	if err != nil || !ok || rehash {
		t.Fatalf("Verify(correct) = %v, %v, %v", ok, rehash, err)
	}
	ok, _, err = h.Verify(ctx, pw(t, "correct horse batterY"), hash)
	if err != nil || ok {
		t.Fatalf("Verify(wrong) = %v, %v", ok, err)
	}
}

func TestSaltsAreUnique(t *testing.T) {
	h := newHasher(t, testParams)
	a, _ := h.Hash(context.Background(), pw(t, "same password here"))
	b, _ := h.Hash(context.Background(), pw(t, "same password here"))
	if a == b {
		t.Fatal("two hashes of the same password are identical (salt not random)")
	}
}

func TestNeedsRehashWhenParamsChange(t *testing.T) {
	ctx := context.Background()
	old := newHasher(t, testParams)
	hash, _ := old.Hash(ctx, pw(t, "upgrade me please"))

	stronger := newHasher(t, Params{MemoryKiB: minMemoryKiB * 2, Iterations: 3, Parallelism: 1})
	ok, rehash, err := stronger.Verify(ctx, pw(t, "upgrade me please"), hash)
	if err != nil || !ok || !rehash {
		t.Fatalf("Verify with stronger params = ok %v rehash %v err %v; want ok and rehash", ok, rehash, err)
	}
}

func TestMalformedHashesRejectedWithoutComputing(t *testing.T) {
	h := newHasher(t, testParams)
	good, _ := h.Hash(context.Background(), pw(t, "valid password x"))
	parts := strings.Split(string(good), "$")
	salt, key := parts[4], parts[5]

	cases := map[string]string{
		"empty":            "",
		"bcrypt":           "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz0123",
		"argon2i":          "$argon2i$v=19$m=19456,t=2,p=1$" + salt + "$" + key,
		"wrong version":    "$argon2id$v=16$m=19456,t=2,p=1$" + salt + "$" + key,
		"missing part":     "$argon2id$v=19$m=19456,t=2,p=1$" + salt,
		"extra part":       string(good) + "$x",
		"params order":     "$argon2id$v=19$t=2,m=19456,p=1$" + salt + "$" + key,
		"non numeric":      "$argon2id$v=19$m=abc,t=2,p=1$" + salt + "$" + key,
		"memory bomb":      "$argon2id$v=19$m=4194304,t=2,p=1$" + salt + "$" + key, // 4 GiB
		"too many passes":  "$argon2id$v=19$m=19456,t=1000,p=1$" + salt + "$" + key,
		"too weak":         "$argon2id$v=19$m=1024,t=1,p=1$" + salt + "$" + key,
		"zero parallelism": "$argon2id$v=19$m=19456,t=2,p=0$" + salt + "$" + key,
		"huge parallelism": "$argon2id$v=19$m=19456,t=2,p=255$" + salt + "$" + key,
		"bad salt b64":     "$argon2id$v=19$m=19456,t=2,p=1$!!!$" + key,
		"short salt":       "$argon2id$v=19$m=19456,t=2,p=1$YWJj$" + key,
		"short key":        "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$YWJj",
	}
	for name, hash := range cases {
		start := time.Now()
		ok, _, err := h.Verify(context.Background(), pw(t, "valid password x"), domain.PasswordHash(hash))
		if ok || !errors.Is(err, ErrMalformedHash) {
			t.Errorf("%s: ok=%v err=%v, want ErrMalformedHash", name, ok, err)
		}
		if d := time.Since(start); d > 50*time.Millisecond {
			t.Errorf("%s: rejection took %v (must not run the KDF)", name, d)
		}
	}
}

func FuzzDecode(f *testing.F) {
	f.Add("$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5")
	f.Add("$argon2id$v=19$m=4294967295,t=4294967295,p=255$$")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		p, err := decode(s)
		if err != nil {
			return
		}
		// Anything accepted must be within the cost limits.
		if p.params.Validate() != nil || len(p.salt) < minSaltLen || len(p.key) < minKeyLen ||
			len(p.salt) > maxSaltLen || len(p.key) > maxKeyLen {
			t.Fatalf("decode accepted out-of-range hash %q: %+v", s, p.params)
		}
	})
}

func TestParamsValidation(t *testing.T) {
	if err := DefaultParams.Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	for _, p := range []Params{
		{MemoryKiB: 1024, Iterations: 3, Parallelism: 1},
		{MemoryKiB: 64 * 1024, Iterations: 1, Parallelism: 1},
		{MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 0},
		{MemoryKiB: 1 << 20, Iterations: 3, Parallelism: 1},
	} {
		if _, err := New(p, 1); err == nil {
			t.Errorf("New(%+v) accepted", p)
		}
	}
	if _, err := New(testParams, 0); err == nil {
		t.Error("maxConcurrent 0 accepted")
	}
}

func TestEmptyPasswordCannotBeHashed(t *testing.T) {
	if _, err := newHasher(t, testParams).Hash(context.Background(), domain.Password{}); err == nil {
		t.Fatal("empty password hashed")
	}
}

func TestConcurrencyIsBounded(t *testing.T) {
	h, err := New(testParams, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Observe the semaphore while 8 goroutines hash concurrently.
	var maxSeen, wg = atomic.Int64{}, sync.WaitGroup{}
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				if n := int64(len(h.sem)); n > maxSeen.Load() {
					maxSeen.Store(n)
				}
			}
		}
	}()
	for range 8 {
		wg.Go(func() { _, _ = h.Hash(context.Background(), pw(t, "concurrent password")) })
	}
	wg.Wait()
	close(stop)
	if got := maxSeen.Load(); got > 2 {
		t.Fatalf("observed %d concurrent computations, limit is 2", got)
	}
}

func TestContextCancelWhileWaiting(t *testing.T) {
	h, _ := New(testParams, 1)
	h.sem <- struct{}{} // occupy the only slot
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, pw(t, "waiting password")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	<-h.sem
}

func TestVerifyDummyTakesComparableTime(t *testing.T) {
	ctx := context.Background()
	h := newHasher(t, testParams)
	hash, _ := h.Hash(ctx, pw(t, "real user password"))

	measure := func(fn func()) time.Duration {
		const n = 5
		start := time.Now()
		for range n {
			fn()
		}
		return time.Since(start) / n
	}
	verify := measure(func() { _, _, _ = h.Verify(ctx, pw(t, "wrong guess xx"), hash) })
	dummy := measure(func() { h.VerifyDummy(ctx, pw(t, "wrong guess xx")) })
	ratio := float64(dummy) / float64(verify)
	if ratio < 0.5 || ratio > 2 {
		t.Errorf("VerifyDummy %v vs Verify %v (ratio %.2f): timing differs enough to reveal accounts", dummy, verify, ratio)
	}
}

// BenchmarkHashDefault measures the production cost (target ≈100–250 ms per hash, security.md).
func BenchmarkHashDefault(b *testing.B) {
	h := newHasher(b, DefaultParams)
	p := pw(b, "benchmark password")
	for b.Loop() {
		if _, err := h.Hash(context.Background(), p); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHashCalibration compares iteration counts at 64 MiB to calibrate ARGON2_ITERATIONS
// on the target machine (the QA droplet in Phase 3).
func BenchmarkHashCalibration(b *testing.B) {
	for _, iters := range []uint32{2, 3, 4, 5, 6} {
		b.Run(fmt.Sprintf("m=64MiB_t=%d_p=2", iters), func(b *testing.B) {
			h := newHasher(b, Params{MemoryKiB: 64 * 1024, Iterations: iters, Parallelism: 2})
			p := pw(b, "benchmark password")
			for b.Loop() {
				if _, err := h.Hash(context.Background(), p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
