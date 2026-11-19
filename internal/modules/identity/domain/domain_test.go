package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseEmail(t *testing.T) {
	valid := map[string]string{
		"ana@example.com":            "ana@example.com",
		"  Ana.Perez@Example.COM  ":  "ana.perez@example.com",
		"ana+citas@sub.example.co":   "ana+citas@sub.example.co",
		"a_b-c@my-domain.example.io": "a_b-c@my-domain.example.io",
	}
	for in, want := range valid {
		got, err := ParseEmail(in)
		if err != nil || got.String() != want {
			t.Errorf("ParseEmail(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	invalid := []string{
		"", "   ", "ana", "ana@", "@example.com", "ana@localhost", "ana@example",
		"Ana <ana@example.com>", "ana@example.com, bob@example.com", "ana @example.com",
		"ana@-example.com", "ana@example-.com", "ana@exa_mple.com", "ana@example..com",
		"ana@.example.com", "ana@example.com.", "ana@ex ample.com", "\"ana\"@example.com\r\n",
		strings.Repeat("a", 65) + "@example.com",
		"ana@" + strings.Repeat("a", 250) + ".com",
	}
	for _, in := range invalid {
		if got, err := ParseEmail(in); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("ParseEmail(%q) = %q, %v; want ErrInvalidEmail", in, got, err)
		}
	}
}

func FuzzParseEmail(f *testing.F) {
	for _, s := range []string{"ana@example.com", "A@B.CO", "x <y@z.com>", "a@b", "\x00@a.co"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		e, err := ParseEmail(s)
		if err != nil {
			return
		}
		// Accepted emails are normalized, bounded and stable under re-parsing.
		if len(e) > MaxEmailLength || e.String() != strings.ToLower(e.String()) || strings.ContainsAny(e.String(), " \t\r\n<>,") {
			t.Fatalf("accepted unsafe email %q", e)
		}
		again, err := ParseEmail(e.String())
		if err != nil || again != e {
			t.Fatalf("re-parse of %q = %q, %v", e, again, err)
		}
	})
}

func TestNewPasswordPolicy(t *testing.T) {
	cases := []struct {
		in   string
		want error
	}{
		{"corto12", ErrPasswordTooShort},
		{"ñandú12", ErrPasswordTooShort}, // 7 runes, 9 bytes: counted in runes
		{"12345678", ErrPasswordTooCommon},
		{"Password1", ErrPasswordTooCommon},
		{"aaaaaaaa", ErrPasswordTooCommon},
		{"tijera78", nil}, // exactly the minimum
		{"ñandú123", nil}, // 8 runes
		{strings.Repeat("x", MaxPasswordLength) + "y", ErrPasswordTooLong},
		{"password123", ErrPasswordTooCommon},
		{"PassWord123", ErrPasswordTooCommon}, // case-insensitive blocklist
		{"Contraseña123", ErrPasswordTooCommon},
		{"aaaaaaaaaaaa", ErrPasswordTooCommon}, // single repeated character
		{"una frase larga y rara", nil},
		{"ñandú12345", nil},
		{strings.Repeat("z", MaxPasswordLength-1) + "q", nil},
	}
	for _, tc := range cases {
		_, err := NewPassword(tc.in)
		if !errors.Is(err, tc.want) {
			t.Errorf("NewPassword(%q) err = %v, want %v", tc.in, err, tc.want)
		}
	}
}

func TestPasswordForVerification(t *testing.T) {
	if p, err := PasswordForVerification("short"); err != nil || p.IsZero() {
		t.Errorf("login with legacy short password must be verifiable: %v", err)
	}
	if _, err := PasswordForVerification(strings.Repeat("x", MaxPasswordLength+1)); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("oversized login password accepted: %v", err)
	}
}

func TestPasswordNeverPrintsItsValue(t *testing.T) {
	const secret = "una-clave-muy-secreta"
	p, err := NewPassword(secret)
	if err != nil {
		t.Fatal(err)
	}
	type wrapper struct{ P Password }

	var logBuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&logBuf, nil)).Info("x", slog.Any("password", p), slog.Any("wrapped", wrapper{p}))
	js, _ := json.Marshal(wrapper{p})

	outputs := []string{
		fmt.Sprint(p), fmt.Sprintf("%v %+v %#v %s %q", p, p, p, p, p),
		fmt.Sprintf("%v %+v %#v", wrapper{p}, wrapper{p}, wrapper{p}),
		logBuf.String(), string(js),
	}
	for _, out := range outputs {
		if strings.Contains(out, secret) {
			t.Errorf("password leaked: %s", out)
		}
	}
	if string(p.Reveal()) != secret {
		t.Error("Reveal() must return the plaintext for the hasher")
	}
}

func TestNewUserInvariants(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	id := uuid.Must(uuid.NewV7())
	email, _ := ParseEmail("ana@example.com")

	u, err := NewUser(id, email, "  Ana   María  Pérez ", "$argon2id$x", now)
	if err != nil {
		t.Fatal(err)
	}
	if u.FullName() != "Ana María Pérez" || u.Status() != UserPendingVerification || u.EmailVerifiedAt() != nil || u.IsPlatformAdmin() ||
		u.ID() != id || u.Email() != email || !u.CreatedAt().Equal(now) || !u.UpdatedAt().Equal(now) || u.PasswordHash() != "$argon2id$x" {
		t.Errorf("unexpected user: %+v", u)
	}

	for name, tc := range map[string]struct {
		email Email
		name  string
		hash  PasswordHash
		want  error
	}{
		"empty name":        {email, "   ", "h", ErrInvalidFullName},
		"control char name": {email, "Ana\x00Pérez", "h", ErrInvalidFullName},
		"too long name":     {email, strings.Repeat("a", MaxFullNameLength+1), "h", ErrInvalidFullName},
		"missing email":     {"", "Ana", "h", ErrInvalidEmail},
		"missing hash":      {email, "Ana", "", ErrEmptyPasswordHash},
	} {
		if _, err := NewUser(id, tc.email, tc.name, tc.hash, now); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestUserLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	email, _ := ParseEmail("ana@example.com")
	u, _ := NewUser(uuid.Must(uuid.NewV7()), email, "Ana", "h1", now)

	if err := u.CanAuthenticate(); !errors.Is(err, ErrUserNotVerified) {
		t.Fatalf("unverified user must not authenticate: %v", err)
	}
	later := now.Add(time.Hour)
	if err := u.VerifyEmail(later); err != nil || u.Status() != UserActive || u.EmailVerifiedAt() == nil || !u.EmailVerifiedAt().Equal(later) {
		t.Fatalf("VerifyEmail: %v (status %s)", err, u.Status())
	}
	if err := u.VerifyEmail(later.Add(time.Hour)); err != nil || !u.EmailVerifiedAt().Equal(later) {
		t.Fatalf("VerifyEmail must be idempotent and keep the first instant: %v", err)
	}
	if err := u.CanAuthenticate(); err != nil {
		t.Fatalf("verified user cannot authenticate: %v", err)
	}
	if err := u.ChangePasswordHash("h2", later); err != nil || u.PasswordHash() != "h2" || !u.UpdatedAt().Equal(later) {
		t.Errorf("ChangePasswordHash failed: %v", err)
	}
	if err := u.ChangePasswordHash("", later); !errors.Is(err, ErrEmptyPasswordHash) {
		t.Errorf("empty hash accepted: %v", err)
	}
	u.Suspend(later)
	if err := u.CanAuthenticate(); !errors.Is(err, ErrUserSuspended) {
		t.Errorf("suspended user: %v", err)
	}
	if err := u.VerifyEmail(later); !errors.Is(err, ErrUserSuspended) {
		t.Errorf("suspended user verified: %v", err)
	}
	u.Reactivate(later)
	if err := u.CanAuthenticate(); err != nil {
		t.Errorf("reactivated user: %v", err)
	}

	pending := RehydrateUser(u.ID(), email, "Ana", "h", UserPendingVerification, true, nil, now, now)
	if err := pending.CanAuthenticate(); !errors.Is(err, ErrUserNotVerified) || !pending.IsPlatformAdmin() {
		t.Errorf("pending user: %v", err)
	}
}

func TestParseStatusAndRole(t *testing.T) {
	for _, s := range []string{"ACTIVE", "SUSPENDED", "PENDING_VERIFICATION"} {
		if _, err := ParseUserStatus(s); err != nil {
			t.Errorf("ParseUserStatus(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "active", "ROOT"} {
		if _, err := ParseUserStatus(s); !errors.Is(err, ErrInvalidStatus) {
			t.Errorf("ParseUserStatus(%q) accepted", s)
		}
	}
	for _, s := range []string{"OWNER", "MANAGER", "PROFESSIONAL"} {
		if _, err := ParseRole(s); err != nil {
			t.Errorf("ParseRole(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "owner", "ADMIN", "CUSTOMER"} {
		if _, err := ParseRole(s); !errors.Is(err, ErrInvalidRole) {
			t.Errorf("ParseRole(%q) accepted", s)
		}
	}
}

func TestTokens(t *testing.T) {
	entropy := bytes.Repeat([]byte{0xAB}, TokenEntropyBytes)
	tok, err := NewToken(TokenPrefixEmailVerification, entropy)
	if err != nil {
		t.Fatal(err)
	}
	raw := tok.Reveal()
	if !strings.HasPrefix(raw, "bp_ev_") || len(raw) != len("bp_ev_")+43 {
		t.Fatalf("raw token %q has wrong shape", raw)
	}
	if len(tok.Hash()) != 32 || bytes.Equal(tok.Hash(), []byte(raw)) {
		t.Fatal("Hash must be a 32-byte SHA-256 distinct from the raw token")
	}

	parsed, err := ParseToken(TokenPrefixEmailVerification, raw)
	if err != nil || !bytes.Equal(parsed.Hash(), tok.Hash()) {
		t.Fatalf("ParseToken round trip: %v", err)
	}
	for _, bad := range []string{
		"", raw[:len(raw)-1], raw + "A", "bp_s_" + raw[len("bp_ev_"):], // wrong kind
		"bp_ev_" + strings.Repeat("!", 43),
	} {
		if _, err := ParseToken(TokenPrefixEmailVerification, bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("ParseToken(%q) accepted", bad)
		}
	}
	if _, err := NewToken(TokenPrefixSession, entropy[:16]); err == nil {
		t.Error("short entropy accepted")
	}
	if _, err := NewToken("", entropy); err == nil {
		t.Error("empty prefix accepted")
	}

	var logBuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&logBuf, nil)).Info("x", slog.Any("token", tok))
	js, _ := json.Marshal(struct{ T Token }{tok})
	for _, out := range []string{fmt.Sprintf("%v %+v %#v %s", tok, tok, tok, tok), logBuf.String(), string(js)} {
		if strings.Contains(out, raw[6:]) {
			t.Errorf("token leaked: %s", out)
		}
	}
}
