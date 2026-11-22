package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testDBURL = "postgres://app:s3cr3t-pass@localhost:5432/booking?sslmode=disable"

// envFrom returns a getenv backed by m. DATABASE_URL defaults to a valid value unless the
// map sets the key (an empty string means "unset").
func envFrom(m map[string]string) func(string) string {
	return func(k string) string {
		if v, ok := m[k]; ok {
			return v
		}
		if k == "DATABASE_URL" {
			return testDBURL
		}
		return ""
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{"APP_ENV": "development"}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q", cfg.Env)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v", cfg.LogLevel)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("Addr = %q", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v", cfg.HTTP.ReadHeaderTimeout)
	}
	if cfg.HTTP.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes = %d", cfg.HTTP.MaxBodyBytes)
	}
	if len(cfg.HTTP.CORSAllowedOrigins) != 0 {
		t.Errorf("CORSAllowedOrigins = %v", cfg.HTTP.CORSAllowedOrigins)
	}
	if cfg.Database.URL != testDBURL || cfg.Database.MigratorURL != testDBURL {
		t.Errorf("Database URLs not loaded / migrator fallback not applied")
	}
	if cfg.Database.MaxConns != 10 || cfg.Database.ConnectTimeout != 5*time.Second {
		t.Errorf("Database pool defaults = %d, %v", cfg.Database.MaxConns, cfg.Database.ConnectTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{
		"APP_ENV":                  "qa",
		"LOG_LEVEL":                "debug",
		"HTTP_ADDR":                "127.0.0.1:9000",
		"HTTP_READ_TIMEOUT":        "3s",
		"HTTP_MAX_BODY_BYTES":      "2048",
		"CORS_ALLOWED_ORIGINS":     " https://business.example.com , https://admin.example.com:8443 ",
		"HTTP_SHUTDOWN_TIMEOUT":    "1m",
		"HTTP_READ_HEADER_TIMEOUT": "2s",
		"DATABASE_MIGRATOR_URL":    "postgresql://migrator:x@db:5432/booking",
		"DB_MAX_CONNS":             "25",
		"DB_CONNECT_TIMEOUT":       "2s",
		"SMTP_TLS":                 "starttls",
		"PUBLIC_APP_URL":           "https://qa.example.com",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LogLevel != slog.LevelDebug || cfg.HTTP.Addr != "127.0.0.1:9000" ||
		cfg.HTTP.ReadTimeout != 3*time.Second || cfg.HTTP.MaxBodyBytes != 2048 ||
		cfg.HTTP.ShutdownTimeout != time.Minute || cfg.HTTP.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("unexpected HTTP config: %+v", cfg.HTTP)
	}
	want := []string{"https://business.example.com", "https://admin.example.com:8443"}
	if strings.Join(cfg.HTTP.CORSAllowedOrigins, "|") != strings.Join(want, "|") {
		t.Errorf("CORSAllowedOrigins = %v, want %v", cfg.HTTP.CORSAllowedOrigins, want)
	}
	if cfg.Database.MigratorURL != "postgresql://migrator:x@db:5432/booking" ||
		cfg.Database.MaxConns != 25 || cfg.Database.ConnectTimeout != 2*time.Second {
		t.Errorf("unexpected database config (MaxConns=%d)", cfg.Database.MaxConns)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing APP_ENV", map[string]string{}, "APP_ENV: is required"},
		{"unknown APP_ENV", map[string]string{"APP_ENV": "staging"}, "unsupported value"},
		{"bad log level", map[string]string{"APP_ENV": "test", "LOG_LEVEL": "loud"}, "LOG_LEVEL"},
		{"bad duration", map[string]string{"APP_ENV": "test", "HTTP_READ_TIMEOUT": "fast"}, "HTTP_READ_TIMEOUT"},
		{"negative duration", map[string]string{"APP_ENV": "test", "HTTP_IDLE_TIMEOUT": "-1s"}, "HTTP_IDLE_TIMEOUT"},
		{"bad body size", map[string]string{"APP_ENV": "test", "HTTP_MAX_BODY_BYTES": "1MB"}, "HTTP_MAX_BODY_BYTES"},
		{"zero body size", map[string]string{"APP_ENV": "test", "HTTP_MAX_BODY_BYTES": "0"}, "greater than zero"},
		{"wildcard origin", map[string]string{"APP_ENV": "development", "CORS_ALLOWED_ORIGINS": "*"}, "wildcard"},
		{"origin with path", map[string]string{"APP_ENV": "development", "CORS_ALLOWED_ORIGINS": "https://a.com/app"}, "invalid origin"},
		{"origin without scheme", map[string]string{"APP_ENV": "development", "CORS_ALLOWED_ORIGINS": "a.com"}, "invalid origin"},
		{"http origin in production", map[string]string{"APP_ENV": "production", "CORS_ALLOWED_ORIGINS": "http://a.com"}, "must use https"},
		{"missing DATABASE_URL", map[string]string{"APP_ENV": "test", "DATABASE_URL": ""}, "DATABASE_URL: is required"},
		{"non-postgres DATABASE_URL", map[string]string{"APP_ENV": "test", "DATABASE_URL": "mysql://u:p@h/db"}, "DATABASE_URL: must be a postgres"},
		{"DATABASE_URL without db name", map[string]string{"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@h:5432"}, "DATABASE_URL: must be a postgres"},
		{"bad migrator URL", map[string]string{"APP_ENV": "test", "DATABASE_MIGRATOR_URL": "nope"}, "DATABASE_MIGRATOR_URL"},
		{"pool too large", map[string]string{"APP_ENV": "test", "DB_MAX_CONNS": "500"}, "DB_MAX_CONNS: must be between"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(envFrom(tt.env))
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadNeverLeaksDatabaseCredentials(t *testing.T) {
	_, err := Load(envFrom(map[string]string{
		"APP_ENV":               "test",
		"DATABASE_URL":          "mysql://admin:topsecret@h/db",
		"DATABASE_MIGRATOR_URL": "postgres://mig:alsosecret@h", // no db name
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, secret := range []string{"topsecret", "alsosecret"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error leaks credentials: %q", err)
		}
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := Load(envFrom(map[string]string{"LOG_LEVEL": "loud", "HTTP_READ_TIMEOUT": "x", "DATABASE_URL": ""}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"APP_ENV", "LOG_LEVEL", "HTTP_READ_TIMEOUT", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q missing %s", err, key)
		}
	}
}

func TestHTTPOriginAllowedInDevelopment(t *testing.T) {
	if _, err := Load(envFrom(map[string]string{
		"APP_ENV": "development", "CORS_ALLOWED_ORIGINS": "http://localhost:3000",
	})); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestArgon2Config(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{"APP_ENV": "test"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Argon2.MemoryKiB != 64*1024 || cfg.Argon2.Iterations != 3 || cfg.Argon2.Parallelism != 2 || cfg.Argon2.MaxConcurrent != 4 {
		t.Errorf("argon2 defaults = %+v", cfg.Argon2)
	}
	for key, val := range map[string]string{
		"ARGON2_MEMORY_KIB":     "1024", // below OWASP minimum
		"ARGON2_ITERATIONS":     "1",
		"ARGON2_PARALLELISM":    "0",
		"ARGON2_MAX_CONCURRENT": "1000",
	} {
		if _, err := Load(envFrom(map[string]string{"APP_ENV": "test", key: val})); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%s accepted (err %v)", key, val, err)
		}
	}
}

func TestSMTPAndPublicURLConfig(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{"APP_ENV": "development"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTP.Port != 1025 || cfg.SMTP.TLSMode != "none" || cfg.PublicAppURL != "http://localhost:3000" {
		t.Errorf("dev defaults = %+v %q", cfg.SMTP, cfg.PublicAppURL)
	}
	if _, err := Load(envFrom(map[string]string{
		"APP_ENV": "qa", "SMTP_TLS": "starttls", "PUBLIC_APP_URL": "https://qa.example.com",
	})); err != nil {
		t.Errorf("valid QA config rejected: %v", err)
	}
	// QA uses Mailpit on the internal network without TLS.
	if _, err := Load(envFrom(map[string]string{
		"APP_ENV": "qa", "SMTP_HOST": "mailpit", "SMTP_TLS": "none", "PUBLIC_APP_URL": "https://qa.example.com",
	})); err != nil {
		t.Errorf("QA with Mailpit rejected: %v", err)
	}
	for name, env := range map[string]map[string]string{
		"prod without TLS":   {"APP_ENV": "production", "PUBLIC_APP_URL": "https://app.example.com"},
		"prod http app url":  {"APP_ENV": "production", "SMTP_TLS": "tls", "PUBLIC_APP_URL": "http://app.example.com"},
		"unknown TLS":        {"APP_ENV": "test", "SMTP_TLS": "ssl"},
		"user without pass":  {"APP_ENV": "test", "SMTP_USERNAME": "u"},
		"relative app url":   {"APP_ENV": "test", "PUBLIC_APP_URL": "/app"},
		"app url with query": {"APP_ENV": "test", "PUBLIC_APP_URL": "https://a.com/?x=1"},
		"port out of range":  {"APP_ENV": "test", "SMTP_PORT": "70000"},
	} {
		if _, err := Load(envFrom(env)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRateLimitConfig(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{"APP_ENV": "test"}))
	if err != nil {
		t.Fatal(err)
	}
	want := RateLimit{APIPerMinute: 300, APIBurst: 100, AuthPerMinute: 10, AuthBurst: 10, MaxKeys: 100_000, LoginBackoffThreshold: 5, LoginBackoffMax: 15 * time.Minute}
	if cfg.RateLimit != want {
		t.Errorf("defaults = %+v", cfg.RateLimit)
	}
	cfg, err = Load(envFrom(map[string]string{"APP_ENV": "test", "RATE_LIMIT_AUTH_PER_MINUTE": "20", "LOGIN_BACKOFF_MAX": "1h"}))
	if err != nil || cfg.RateLimit.AuthPerMinute != 20 || cfg.RateLimit.LoginBackoffMax != time.Hour {
		t.Errorf("overrides = %+v, %v", cfg.RateLimit, err)
	}
	for key, val := range map[string]string{
		"RATE_LIMIT_API_PER_MINUTE":  "0",
		"RATE_LIMIT_AUTH_PER_MINUTE": "100000",
		"RATE_LIMIT_AUTH_BURST":      "0",
		"RATE_LIMIT_MAX_KEYS":        "10",
		"LOGIN_BACKOFF_THRESHOLD":    "0",
		"LOGIN_BACKOFF_MAX":          "100ms",
	} {
		if _, err := Load(envFrom(map[string]string{"APP_ENV": "test", key: val})); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%s accepted (err %v)", key, val, err)
		}
	}
}

func TestTrustedProxiesConfig(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{"APP_ENV": "test"}))
	if err != nil || len(cfg.HTTP.TrustedProxies) != 0 {
		t.Fatalf("default must trust nobody: %v %v", cfg.HTTP.TrustedProxies, err)
	}
	cfg, err = Load(envFrom(map[string]string{"APP_ENV": "test", "TRUSTED_PROXIES": "172.30.0.10, 10.0.0.0/8,fd00::1"}))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, p := range cfg.HTTP.TrustedProxies {
		got = append(got, p.String())
	}
	if strings.Join(got, " ") != "172.30.0.10/32 10.0.0.0/8 fd00::1/128" {
		t.Fatalf("prefixes = %v", got)
	}
	for _, bad := range []string{"0.0.0.0/0", "::/0", "not-an-ip", "10.0.0.0/33"} {
		if _, err := Load(envFrom(map[string]string{"APP_ENV": "test", "TRUSTED_PROXIES": bad})); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
			t.Errorf("%q accepted (err %v)", bad, err)
		}
	}
}
