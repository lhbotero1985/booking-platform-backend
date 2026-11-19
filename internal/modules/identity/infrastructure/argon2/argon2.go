// Package argon2 implements application.PasswordHasher with Argon2id (ADR-0006) using the PHC
// string format: $argon2id$v=19$m=<KiB>,t=<iterations>,p=<parallelism>$<salt>$<hash>.
package argon2

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	xargon2 "golang.org/x/crypto/argon2"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

const (
	saltLen = 16
	keyLen  = 32
)

// Limits accepted when parsing a stored hash. They bound the cost an attacker who can write a
// hash (or a corrupted row) could impose on Verify.
const (
	minMemoryKiB = 19 * 1024  // OWASP minimum (19 MiB)
	maxMemoryKiB = 256 * 1024 // 256 MiB
	minIter      = 2
	maxIter      = 10
	maxParallel  = 16
	minSaltLen   = 8
	maxSaltLen   = 64
	minKeyLen    = 16
	maxKeyLen    = 64
)

// ErrMalformedHash is returned for hashes that are not valid Argon2id PHC strings or exceed the
// accepted parameter limits.
var ErrMalformedHash = errors.New("argon2: malformed or unsupported hash")

// Params are the cost parameters for new hashes.
type Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
}

// DefaultParams are 64 MiB, 3 passes and 2 lanes, above the OWASP minimum (19 MiB, t=2, p=1).
var DefaultParams = Params{MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 2}

// Validate checks p against the accepted limits.
func (p Params) Validate() error {
	if p.MemoryKiB < minMemoryKiB || p.MemoryKiB > maxMemoryKiB ||
		p.Iterations < minIter || p.Iterations > maxIter ||
		p.Parallelism < 1 || p.Parallelism > maxParallel {
		return fmt.Errorf("argon2: params out of range (m=%d KiB, t=%d, p=%d)", p.MemoryKiB, p.Iterations, p.Parallelism)
	}
	return nil
}

// Hasher hashes with the configured Params. A semaphore bounds concurrent computations: each
// one allocates MemoryKiB, so unbounded parallel logins could exhaust memory.
type Hasher struct {
	params Params
	sem    chan struct{}
	dummy  domain.PasswordHash
}

// New builds a Hasher allowing at most maxConcurrent simultaneous computations.
func New(params Params, maxConcurrent int) (*Hasher, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if maxConcurrent < 1 {
		return nil, errors.New("argon2: maxConcurrent must be >= 1")
	}
	h := &Hasher{params: params, sem: make(chan struct{}, maxConcurrent)}
	// Dummy hash with the current params for timing equalization (VerifyDummy).
	salt := make([]byte, saltLen)
	_, _ = rand.Read(salt)
	h.dummy = encode(params, salt, xargon2.IDKey([]byte("dummy-password"), salt, params.Iterations, params.MemoryKiB, params.Parallelism, keyLen))
	return h, nil
}

func (h *Hasher) acquire(ctx context.Context) error {
	select {
	case h.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Hasher) release() { <-h.sem }

// Hash implements application.PasswordHasher.
func (h *Hasher) Hash(ctx context.Context, p domain.Password) (domain.PasswordHash, error) {
	if p.IsZero() {
		return "", errors.New("argon2: empty password")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2: salt: %w", err)
	}
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	key := xargon2.IDKey(p.Reveal(), salt, h.params.Iterations, h.params.MemoryKiB, h.params.Parallelism, keyLen)
	return encode(h.params, salt, key), nil
}

// Verify implements application.PasswordHasher.
func (h *Hasher) Verify(ctx context.Context, p domain.Password, hash domain.PasswordHash) (bool, bool, error) {
	ph, err := decode(string(hash))
	if err != nil {
		return false, false, err
	}
	if err := h.acquire(ctx); err != nil {
		return false, false, err
	}
	defer h.release()
	key := xargon2.IDKey(p.Reveal(), ph.salt, ph.params.Iterations, ph.params.MemoryKiB, ph.params.Parallelism, uint32(len(ph.key))) //nolint:gosec // len bounded by maxKeyLen
	if subtle.ConstantTimeCompare(key, ph.key) != 1 {
		return false, false, nil
	}
	needsRehash := ph.params != h.params || len(ph.salt) != saltLen || len(ph.key) != keyLen
	return true, needsRehash, nil
}

// VerifyDummy implements application.PasswordHasher.
func (h *Hasher) VerifyDummy(ctx context.Context, p domain.Password) {
	_, _, _ = h.Verify(ctx, p, h.dummy)
}

type phc struct {
	params Params
	salt   []byte
	key    []byte
}

func encode(p Params, salt, key []byte) domain.PasswordHash {
	b64 := base64.RawStdEncoding
	return domain.PasswordHash(fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		xargon2.Version, p.MemoryKiB, p.Iterations, p.Parallelism, b64.EncodeToString(salt), b64.EncodeToString(key)))
}

// decode parses a PHC string strictly and enforces the parameter limits before any expensive
// computation happens.
func decode(s string) (phc, error) {
	parts := strings.Split(s, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", xargon2.Version) {
		return phc{}, ErrMalformedHash
	}
	params, err := parseParams(parts[3])
	if err != nil {
		return phc{}, err
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < minSaltLen || len(salt) > maxSaltLen {
		return phc{}, ErrMalformedHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) < minKeyLen || len(key) > maxKeyLen {
		return phc{}, ErrMalformedHash
	}
	return phc{params: params, salt: salt, key: key}, nil
}

func parseParams(s string) (Params, error) {
	fields := strings.Split(s, ",")
	if len(fields) != 3 {
		return Params{}, ErrMalformedHash
	}
	var vals [3]uint64
	for i, prefix := range []string{"m=", "t=", "p="} {
		v, ok := strings.CutPrefix(fields[i], prefix)
		if !ok {
			return Params{}, ErrMalformedHash
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return Params{}, ErrMalformedHash
		}
		vals[i] = n
	}
	if vals[2] > maxParallel {
		return Params{}, ErrMalformedHash
	}
	p := Params{MemoryKiB: uint32(vals[0]), Iterations: uint32(vals[1]), Parallelism: uint8(vals[2])} //nolint:gosec // parsed with bitSize 32 and checked
	if p.Validate() != nil {
		return Params{}, ErrMalformedHash
	}
	return p, nil
}
