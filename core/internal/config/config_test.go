package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("AITASK_ENV", "")
	t.Setenv("AITASK_SERVICE_NAME", "")
	t.Setenv("AITASK_SERVICE_VERSION", "")
	t.Setenv("AITASK_HTTP_HOST", "")
	t.Setenv("AITASK_HTTP_PORT", "")
	t.Setenv("AITASK_HTTP_READ_TIMEOUT", "")
	t.Setenv("AITASK_HTTP_WRITE_TIMEOUT", "")
	t.Setenv("AITASK_HTTP_IDLE_TIMEOUT", "")
	t.Setenv("AITASK_HTTP_SHUTDOWN_TIMEOUT", "")
	t.Setenv("POSTGRES_HOST", "")
	t.Setenv("POSTGRES_PORT", "")
	t.Setenv("POSTGRES_DB", "")
	t.Setenv("POSTGRES_USER", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DRAGONFLY_HOST", "")
	t.Setenv("DRAGONFLY_PORT", "")
	t.Setenv("DRAGONFLY_PASSWORD", "")
	t.Setenv("DRAGONFLY_URL", "")
	t.Setenv("AGENT_TOKEN_SECRET", "")
	t.Setenv("OPENVIKING_NAMESPACE", "")
	t.Setenv("OPENVIKING_SETTINGS_KEY", "")
	t.Setenv("CONSOLE_OPERATOR_LABEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Environment != defaultEnvironment {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, defaultEnvironment)
	}
	if cfg.Service.Name != defaultServiceName {
		t.Fatalf("Service.Name = %q, want %q", cfg.Service.Name, defaultServiceName)
	}
	if got, want := cfg.Server.Addr(), "0.0.0.0:8080"; got != want {
		t.Fatalf("Server.Addr() = %q, want %q", got, want)
	}
	if got, want := cfg.Postgres.URL, "postgres://aitask:aitask_dev_password@postgres:5432/aitask?sslmode=disable"; got != want {
		t.Fatalf("Postgres.URL = %q, want %q", got, want)
	}
	if got, want := cfg.Dragonfly.URL, "redis://:dragonfly_dev_password@dragonfly:6379/0"; got != want {
		t.Fatalf("Dragonfly.URL = %q, want %q", got, want)
	}
	if got, want := cfg.Security.AgentTokenSecret, defaultAgentSecret; got != want {
		t.Fatalf("AgentTokenSecret = %q, want %q", got, want)
	}
	if got, want := len(cfg.OpenViking.SettingsKey), 32; got != want {
		t.Fatalf("OpenViking.SettingsKey length = %d, want %d", got, want)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv("AITASK_ENV", "production")
	t.Setenv("AITASK_SERVICE_NAME", "aitask-api")
	t.Setenv("AITASK_SERVICE_VERSION", "v1")
	t.Setenv("AITASK_HTTP_HOST", "127.0.0.1")
	t.Setenv("AITASK_HTTP_PORT", "18080")
	t.Setenv("AITASK_HTTP_READ_TIMEOUT", "2s")
	t.Setenv("AITASK_HTTP_WRITE_TIMEOUT", "3s")
	t.Setenv("AITASK_HTTP_IDLE_TIMEOUT", "4s")
	t.Setenv("AITASK_HTTP_SHUTDOWN_TIMEOUT", "5s")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_PORT", "15432")
	t.Setenv("POSTGRES_DB", "aitask_test")
	t.Setenv("POSTGRES_USER", "dbuser")
	t.Setenv("POSTGRES_PASSWORD", "dbpass")
	t.Setenv("DATABASE_URL", "postgres://dbuser:dbpass@db:15432/aitask_test?sslmode=disable")
	t.Setenv("DRAGONFLY_HOST", "cache")
	t.Setenv("DRAGONFLY_PORT", "17379")
	t.Setenv("DRAGONFLY_PASSWORD", "cachepass")
	t.Setenv("DRAGONFLY_URL", "redis://:cachepass@cache:17379/0")
	t.Setenv("AGENT_TOKEN_SECRET", "secret-token")
	t.Setenv("OPENVIKING_NAMESPACE", "aitask-dev")
	settingsKey := []byte("0123456789abcdef0123456789abcdef")
	t.Setenv("OPENVIKING_SETTINGS_KEY", base64.StdEncoding.EncodeToString(settingsKey))
	t.Setenv("CONSOLE_OPERATOR_LABEL", "alice")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.IsProduction() {
		t.Fatalf("IsProduction() = false, want true")
	}
	if got, want := cfg.Server.Addr(), "127.0.0.1:18080"; got != want {
		t.Fatalf("Server.Addr() = %q, want %q", got, want)
	}
	if got, want := cfg.Server.ShutdownTimeout, 5*time.Second; got != want {
		t.Fatalf("ShutdownTimeout = %s, want %s", got, want)
	}
	if got, want := cfg.Postgres.Port, 15432; got != want {
		t.Fatalf("Postgres.Port = %d, want %d", got, want)
	}
	if got, want := cfg.Postgres.URL, "postgres://dbuser:dbpass@db:15432/aitask_test?sslmode=disable"; got != want {
		t.Fatalf("Postgres.URL = %q, want %q", got, want)
	}
	if got, want := cfg.Dragonfly.URL, "redis://:cachepass@cache:17379/0"; got != want {
		t.Fatalf("Dragonfly.URL = %q, want %q", got, want)
	}
	if got, want := cfg.OpenViking.Namespace, "aitask-dev"; got != want {
		t.Fatalf("OpenViking.Namespace = %q, want %q", got, want)
	}
	if got, want := string(cfg.OpenViking.SettingsKey), string(settingsKey); got != want {
		t.Fatalf("OpenViking.SettingsKey = %q, want %q", got, want)
	}
	if got, want := cfg.Security.AgentTokenSecret, "secret-token"; got != want {
		t.Fatalf("AgentTokenSecret = %q, want %q", got, want)
	}
	if got, want := cfg.Console.OperatorLabel, "alice"; got != want {
		t.Fatalf("OperatorLabel = %q, want %q", got, want)
	}
}

func TestServerConfigValidateRejectsInvalidPort(t *testing.T) {
	cfg := ServerConfig{
		Host:            "127.0.0.1",
		Port:            70000,
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
		IdleTimeout:     time.Second,
		ShutdownTimeout: time.Second,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() error = nil, want error")
	}
}

func TestLoadRejectsInvalidEnvironmentValues(t *testing.T) {
	t.Setenv("AITASK_HTTP_PORT", "not-a-port")

	if _, err := Load(); err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
}

func TestLoadRejectsInvalidPostgresPort(t *testing.T) {
	t.Setenv("POSTGRES_PORT", "invalid")

	if _, err := Load(); err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
}

func TestLoadRejectsInvalidOpenVikingSettingsKey(t *testing.T) {
	t.Setenv("OPENVIKING_SETTINGS_KEY", base64.StdEncoding.EncodeToString([]byte("short")))

	if _, err := Load(); err == nil {
		t.Fatalf("Load() error = nil, want error")
	} else if !strings.Contains(err.Error(), "OPENVIKING_SETTINGS_KEY") {
		t.Fatalf("Load() error = %v, want OPENVIKING_SETTINGS_KEY validation error", err)
	}
}
