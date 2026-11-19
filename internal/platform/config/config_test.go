package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
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
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LogLevel != slog.LevelDebug || cfg.HTTP.Addr != "127.0.0.1:9000" ||
		cfg.HTTP.ReadTimeout != 3*time.Second || cfg.HTTP.MaxBodyBytes != 2048 ||
		cfg.HTTP.ShutdownTimeout != time.Minute || cfg.HTTP.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("unexpected config: %+v", cfg)
	}
	want := []string{"https://business.example.com", "https://admin.example.com:8443"}
	if strings.Join(cfg.HTTP.CORSAllowedOrigins, "|") != strings.Join(want, "|") {
		t.Errorf("CORSAllowedOrigins = %v, want %v", cfg.HTTP.CORSAllowedOrigins, want)
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

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := Load(envFrom(map[string]string{"LOG_LEVEL": "loud", "HTTP_READ_TIMEOUT": "x"}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"APP_ENV", "LOG_LEVEL", "HTTP_READ_TIMEOUT"} {
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
