// Package config loads and validates process configuration from environment variables.
//
// Invalid or missing configuration makes Load return an error that lists every problem, so
// the process fails fast at startup instead of misbehaving later.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
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
}

// Config is the full process configuration.
type Config struct {
	Env      Env
	LogLevel slog.Level
	HTTP     HTTP
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
		},
	}

	p.validateOrigins(cfg.Env, cfg.HTTP.CORSAllowedOrigins)
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

func (p *parser) list(key string) []string {
	var out []string
	for item := range strings.SplitSeq(p.getenv(key), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
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
