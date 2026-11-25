// Package config loads and validates process configuration from environment variables.
//
// Invalid or missing configuration makes Load return an error that lists every problem, so
// the process fails fast at startup instead of misbehaving later.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Env identifies the deployment environment.
type Env string

// Supported environments.
const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvQA          Env = "qa"
	EnvProduction  Env = "production"
)

// IsDeployed reports whether the environment runs on shared infrastructure (QA or PROD),
// where stricter rules apply.
func (e Env) IsDeployed() bool {
	return e == EnvQA || e == EnvProduction
}

// HTTP holds HTTP server settings.
type HTTP struct {
	Addr               string
	ReadHeaderTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownTimeout    time.Duration
	MaxBodyBytes       int64
	CORSAllowedOrigins []string
	// TrustedProxies are the reverse proxies (Caddy) whose X-Forwarded-For is believed.
	// Empty: the TCP peer is the client.
	TrustedProxies []netip.Prefix
}

// Database holds PostgreSQL settings. URLs contain credentials: never log them.
type Database struct {
	// URL is used by the runtime (API and worker), with DML-only privileges in QA/PROD.
	URL string
	// MigratorURL is used only by "migrate"; falls back to URL when empty.
	MigratorURL    string
	MaxConns       int32
	ConnectTimeout time.Duration
}

// Worker holds settings for cmd/worker.
type Worker struct {
	// ShutdownTimeout is how long running jobs may finish after SIGTERM before being canceled.
	ShutdownTimeout time.Duration
}

// Argon2 holds password hashing cost (ADR-0006). Bounds match the hasher's accepted limits.
type Argon2 struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	// MaxConcurrent caps simultaneous hash computations (each allocates MemoryKiB).
	MaxConcurrent int
}

// RateLimit configures the in-memory limits (security.md §3). Limits are per client key (IPv4
// address or IPv6 /64) except the login backoff, which is per email+client.
type RateLimit struct {
	// APIPerMinute / APIBurst: general limit for every route except health probes.
	APIPerMinute int
	APIBurst     int
	// AuthPerMinute / AuthBurst: strict limit shared by login, register, email verification and
	// password endpoints.
	AuthPerMinute int
	AuthBurst     int
	// MaxKeys bounds each in-memory table (memory DoS protection).
	MaxKeys int
	// LoginBackoffThreshold consecutive failures start the backoff (1 s, doubling) capped at
	// LoginBackoffMax.
	LoginBackoffThreshold int
	LoginBackoffMax       time.Duration
}

// SMTP configures transactional email. QA/PROD must use TLS (starttls or tls).
type SMTP struct {
	Host     string
	Port     int
	From     string
	Username string
	Password string
	TLSMode  string
}

// Notifications switches the delivery channels per environment (notifications.md §3, T7.1).
type Notifications struct {
	// WhatsAppEnabled: false skips every WhatsApp message (default; DEV).
	WhatsAppEnabled bool
	// WhatsAppAllowlist: when set, the only numbers (E.164) WhatsApp may write to (QA).
	WhatsAppAllowlist []string
	// WhatsApp Cloud API (T7.2). Without an access token the simulated sender is used. Outside
	// production a token requires WhatsAppAllowlist, so QA never writes to a synthetic number.
	WhatsAppAccessToken       string // secret: never logged nor echoed in errors
	WhatsAppPhoneNumberID     string
	WhatsAppBusinessAccountID string
	WhatsAppAPIBaseURL        string // https://graph.facebook.com
	WhatsAppGraphVersion      string // v23.0
}

// WhatsAppConfigured reports whether the Meta credentials are present.
func (n Notifications) WhatsAppConfigured() bool { return n.WhatsAppAccessToken != "" }

// Config is the full process configuration.
type Config struct {
	Env       Env
	LogLevel  slog.Level
	HTTP      HTTP
	Database  Database
	Worker    Worker
	Argon2    Argon2
	SMTP      SMTP
	RateLimit RateLimit
	// Notifications switches WhatsApp and push.
	Notifications Notifications
	// PublicAppURL is the base URL of the client app used in email links (/verify-email, …).
	PublicAppURL string
}

// Load reads configuration using getenv (usually os.Getenv) and validates it.
func Load(getenv func(string) string) (Config, error) {
	p := parser{getenv: getenv}

	cfg := Config{
		Env:      p.env("APP_ENV"),
		LogLevel: p.logLevel("LOG_LEVEL", slog.LevelInfo),
		HTTP: HTTP{
			Addr:               p.str("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout:  p.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:        p.duration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:       p.duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:        p.duration("HTTP_IDLE_TIMEOUT", 120*time.Second),
			ShutdownTimeout:    p.duration("HTTP_SHUTDOWN_TIMEOUT", 20*time.Second),
			MaxBodyBytes:       p.int64("HTTP_MAX_BODY_BYTES", 1<<20),
			CORSAllowedOrigins: p.list("CORS_ALLOWED_ORIGINS"),
			TrustedProxies:     p.prefixes("TRUSTED_PROXIES"),
		},
		Database: Database{
			URL:            p.postgresURL("DATABASE_URL", true),
			MigratorURL:    p.postgresURL("DATABASE_MIGRATOR_URL", false),
			MaxConns:       int32(p.intRange("DB_MAX_CONNS", 10, 1, 100)), //nolint:gosec // bounded to [1,100]
			ConnectTimeout: p.duration("DB_CONNECT_TIMEOUT", 5*time.Second),
		},
		Worker: Worker{
			ShutdownTimeout: p.duration("WORKER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		//nolint:gosec // every value is bounded by intRange before the conversion
		Argon2: Argon2{
			MemoryKiB:     uint32(p.intRange("ARGON2_MEMORY_KIB", 64*1024, 19*1024, 256*1024)),
			Iterations:    uint32(p.intRange("ARGON2_ITERATIONS", 3, 2, 10)),
			Parallelism:   uint8(p.intRange("ARGON2_PARALLELISM", 2, 1, 16)),
			MaxConcurrent: int(p.intRange("ARGON2_MAX_CONCURRENT", 4, 1, 64)),
		},
		SMTP: SMTP{
			Host:     p.str("SMTP_HOST", "localhost"),
			Port:     int(p.intRange("SMTP_PORT", 1025, 1, 65535)),
			From:     p.str("SMTP_FROM", "Reservas <no-reply@booking.local>"),
			Username: p.str("SMTP_USERNAME", ""),
			Password: p.str("SMTP_PASSWORD", ""),
			TLSMode:  p.str("SMTP_TLS", "none"),
		},
		RateLimit: RateLimit{
			APIPerMinute:          int(p.intRange("RATE_LIMIT_API_PER_MINUTE", 300, 10, 100_000)),
			APIBurst:              int(p.intRange("RATE_LIMIT_API_BURST", 100, 1, 100_000)),
			AuthPerMinute:         int(p.intRange("RATE_LIMIT_AUTH_PER_MINUTE", 10, 1, 1_000)),
			AuthBurst:             int(p.intRange("RATE_LIMIT_AUTH_BURST", 10, 1, 1_000)),
			MaxKeys:               int(p.intRange("RATE_LIMIT_MAX_KEYS", 100_000, 1_000, 10_000_000)),
			LoginBackoffThreshold: int(p.intRange("LOGIN_BACKOFF_THRESHOLD", 5, 1, 100)),
			LoginBackoffMax:       p.duration("LOGIN_BACKOFF_MAX", 15*time.Minute),
		},
		PublicAppURL: p.str("PUBLIC_APP_URL", "http://localhost:3000"),
		Notifications: Notifications{
			WhatsAppEnabled:           p.boolean("WHATSAPP_ENABLED", false),
			WhatsAppAllowlist:         p.phones("WHATSAPP_ALLOWLIST"),
			WhatsAppAccessToken:       strings.TrimSpace(p.getenv("WHATSAPP_ACCESS_TOKEN")),
			WhatsAppPhoneNumberID:     p.str("WHATSAPP_PHONE_NUMBER_ID", ""),
			WhatsAppBusinessAccountID: p.str("WHATSAPP_BUSINESS_ACCOUNT_ID", ""),
			WhatsAppAPIBaseURL:        strings.TrimRight(p.str("WHATSAPP_API_BASE_URL", "https://graph.facebook.com"), "/"),
			WhatsAppGraphVersion:      p.str("WHATSAPP_GRAPH_VERSION", "v23.0"),
		},
	}
	if cfg.Database.MigratorURL == "" {
		cfg.Database.MigratorURL = cfg.Database.URL
	}

	p.validateOrigins(cfg.Env, cfg.HTTP.CORSAllowedOrigins)
	p.validateSMTP(cfg.Env, cfg.SMTP)
	p.validatePublicURL(cfg.Env, cfg.PublicAppURL)
	p.validateWhatsApp(cfg.Env, cfg.Notifications)
	if m := cfg.RateLimit.LoginBackoffMax; m < time.Second || m > 24*time.Hour {
		p.fail("LOGIN_BACKOFF_MAX", "must be between 1s and 24h")
	}
	if cfg.HTTP.MaxBodyBytes <= 0 {
		p.fail("HTTP_MAX_BODY_BYTES", "must be greater than zero")
	}

	if err := errors.Join(p.errs...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

type parser struct {
	getenv func(string) string
	errs   []error
}

func (p *parser) fail(key, msg string) {
	p.errs = append(p.errs, fmt.Errorf("%s: %s", key, msg))
}

func (p *parser) str(key, def string) string {
	if v := strings.TrimSpace(p.getenv(key)); v != "" {
		return v
	}
	return def
}

func (p *parser) env(key string) Env {
	v := Env(strings.TrimSpace(p.getenv(key)))
	switch v {
	case EnvDevelopment, EnvTest, EnvQA, EnvProduction:
		return v
	case "":
		p.fail(key, "is required (development, test, qa, production)")
	default:
		p.fail(key, fmt.Sprintf("unsupported value %q (development, test, qa, production)", v))
	}
	return ""
}

func (p *parser) logLevel(key string, def slog.Level) slog.Level {
	v := strings.TrimSpace(p.getenv(key))
	if v == "" {
		return def
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(v)); err != nil {
		p.fail(key, fmt.Sprintf("invalid level %q (debug, info, warn, error)", v))
		return def
	}
	return lvl
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(p.getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		p.fail(key, fmt.Sprintf("invalid positive duration %q (e.g. 15s)", v))
		return def
	}
	return d
}

func (p *parser) int64(key string, def int64) int64 {
	v := strings.TrimSpace(p.getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		p.fail(key, fmt.Sprintf("invalid integer %q", v))
		return def
	}
	return n
}

func (p *parser) intRange(key string, def, minV, maxV int64) int64 {
	n := p.int64(key, def)
	if n < minV || n > maxV {
		p.fail(key, fmt.Sprintf("must be between %d and %d", minV, maxV))
		return def
	}
	return n
}

// postgresURL validates a connection URL without ever echoing it (it carries a password).
func (p *parser) postgresURL(key string, required bool) string {
	v := strings.TrimSpace(p.getenv(key))
	if v == "" {
		if required {
			p.fail(key, "is required (postgres://user:password@host:port/db)")
		}
		return ""
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" || strings.Trim(u.Path, "/") == "" {
		p.fail(key, "must be a postgres:// URL with host and database name (value not shown)")
		return ""
	}
	return v
}

func (p *parser) list(key string) []string {
	var out []string
	for item := range strings.SplitSeq(p.getenv(key), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (p *parser) boolean(key string, def bool) bool {
	switch v := strings.ToLower(strings.TrimSpace(p.getenv(key))); v {
	case "":
		return def
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		p.fail(key, fmt.Sprintf("invalid boolean %q (true or false)", v))
		return def
	}
}

// e164 is an international phone number: + and 8 to 15 digits.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// phones parses a comma-separated list of E.164 numbers; the values are never echoed in errors.
func (p *parser) phones(key string) []string {
	out := p.list(key)
	for i, n := range out {
		if !e164.MatchString(n) {
			p.fail(key, fmt.Sprintf("entry %d is not an E.164 number (+ and 8 to 15 digits)", i+1))
		}
	}
	return out
}

// validateOrigins accepts only exact origins (scheme://host[:port]). Wildcards are rejected
// everywhere because the API uses credentials; plain http is only allowed outside QA/PROD.
func (p *parser) validateOrigins(env Env, origins []string) {
	const key = "CORS_ALLOWED_ORIGINS"
	for _, o := range origins {
		if o == "*" {
			p.fail(key, "wildcard origin is not allowed")
			continue
		}
		u, err := url.Parse(o)
		if err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			p.fail(key, fmt.Sprintf("invalid origin %q (expected scheme://host[:port])", o))
			continue
		}
		switch u.Scheme {
		case "https":
		case "http":
			if env.IsDeployed() {
				p.fail(key, fmt.Sprintf("origin %q must use https in %s", o, env))
			}
		default:
			p.fail(key, fmt.Sprintf("origin %q has unsupported scheme", o))
		}
	}
}

func (p *parser) validateSMTP(env Env, s SMTP) {
	switch s.TLSMode {
	case "none", "starttls", "tls":
	default:
		p.fail("SMTP_TLS", fmt.Sprintf("unsupported value %q (none, starttls, tls)", s.TLSMode))
	}
	// QA delivers to Mailpit on the private compose network (no real email leaves QA, decision of
	// 2026-11-19); production must reach a real provider over TLS.
	if env == EnvProduction && s.TLSMode == "none" {
		p.fail("SMTP_TLS", "must be starttls or tls in production")
	}
	if (s.Username == "") != (s.Password == "") {
		p.fail("SMTP_USERNAME", "SMTP_USERNAME and SMTP_PASSWORD must be set together")
	}
}

func (p *parser) validatePublicURL(env Env, raw string) {
	const key = "PUBLIC_APP_URL"
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" {
		p.fail(key, "must be an absolute http(s) URL without query or fragment")
		return
	}
	if env.IsDeployed() && u.Scheme != "https" {
		p.fail(key, fmt.Sprintf("must use https in %s", env))
	}
}

var (
	metaID       = regexp.MustCompile(`^[0-9]{5,30}$`)
	graphVersion = regexp.MustCompile(`^v[0-9]{1,3}\.[0-9]$`)
	metaToken    = regexp.MustCompile(`^[A-Za-z0-9_-]{20,1000}$`)
)

// validateWhatsApp checks the Meta settings only when a token is set; values are never echoed.
func (p *parser) validateWhatsApp(env Env, n Notifications) {
	if !n.WhatsAppConfigured() {
		if n.WhatsAppPhoneNumberID != "" {
			p.fail("WHATSAPP_ACCESS_TOKEN", "is required when WHATSAPP_PHONE_NUMBER_ID is set")
		}
		return
	}
	if !metaToken.MatchString(n.WhatsAppAccessToken) {
		p.fail("WHATSAPP_ACCESS_TOKEN", "has an invalid format")
	}
	if !metaID.MatchString(n.WhatsAppPhoneNumberID) {
		p.fail("WHATSAPP_PHONE_NUMBER_ID", "must be the numeric phone number id from Meta (not the phone number)")
	}
	if n.WhatsAppBusinessAccountID != "" && !metaID.MatchString(n.WhatsAppBusinessAccountID) {
		p.fail("WHATSAPP_BUSINESS_ACCOUNT_ID", "must be numeric")
	}
	if !graphVersion.MatchString(n.WhatsAppGraphVersion) {
		p.fail("WHATSAPP_GRAPH_VERSION", "must look like v23.0")
	}
	if u, err := url.Parse(n.WhatsAppAPIBaseURL); err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" ||
		(u.Scheme != "https" && (env.IsDeployed() || u.Scheme != "http")) {
		p.fail("WHATSAPP_API_BASE_URL", "must be an https origin (http only outside QA/PROD)")
	}
	if env != EnvProduction && len(n.WhatsAppAllowlist) == 0 {
		p.fail("WHATSAPP_ALLOWLIST", fmt.Sprintf("is required in %s when WhatsApp credentials are set", env))
	}
}

// prefixes parses a comma-separated list of IP addresses or CIDR prefixes. Catch-all prefixes
// are rejected: trusting every peer would let any client choose its own IP.
func (p *parser) prefixes(key string) []netip.Prefix {
	var out []netip.Prefix
	for _, v := range p.list(key) {
		var pfx netip.Prefix
		if strings.Contains(v, "/") {
			parsed, err := netip.ParsePrefix(v)
			if err != nil {
				p.fail(key, fmt.Sprintf("invalid prefix %q", v))
				continue
			}
			pfx = parsed.Masked()
		} else {
			ip, err := netip.ParseAddr(v)
			if err != nil {
				p.fail(key, fmt.Sprintf("invalid address %q", v))
				continue
			}
			pfx = netip.PrefixFrom(ip.Unmap(), ip.Unmap().BitLen())
		}
		if pfx.Bits() == 0 {
			p.fail(key, fmt.Sprintf("%q would trust every client", v))
			continue
		}
		out = append(out, pfx)
	}
	return out
}
