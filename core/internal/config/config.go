package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnvironment     = "development"
	defaultServiceName     = "aitask-backend"
	defaultServiceVersion  = "dev"
	defaultHTTPHost        = "0.0.0.0"
	defaultHTTPPort        = 8080
	defaultReadTimeout     = 10 * time.Second
	defaultWriteTimeout    = 30 * time.Second
	defaultIdleTimeout     = 120 * time.Second
	defaultShutdownTimeout = 10 * time.Second
	defaultPostgresHost    = "postgres"
	defaultPostgresPort    = 5432
	defaultPostgresDB      = "aitask"
	defaultPostgresUser    = "aitask"
	defaultPostgresPass    = "aitask_dev_password"
	defaultDragonflyHost   = "dragonfly"
	defaultDragonflyPort   = 6379
	defaultDragonflyPass   = "dragonfly_dev_password"
	defaultOpenVikingNS    = "aitask"
	defaultAgentSecret     = "dev_only_change_me"
	defaultOperatorLabel   = "local-operator"
	defaultWorkerCron      = "0 5 0 * * *"
)

type Config struct {
	Environment string
	Service     ServiceConfig
	Server      ServerConfig
	Postgres    PostgresConfig
	Dragonfly   DragonflyConfig
	OpenViking  OpenVikingConfig
	Security    SecurityConfig
	Console     ConsoleConfig
	Worker      WorkerConfig
	RateLimit   RateLimitConfig
}

type ServiceConfig struct {
	Name    string
	Version string
}

type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type PostgresConfig struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
	URL      string
}

type DragonflyConfig struct {
	Host     string
	Port     int
	Password string
	URL      string
}

type OpenVikingConfig struct {
	Namespace   string
	SettingsKey []byte
}

type SecurityConfig struct {
	AgentTokenSecret string
}

type ConsoleConfig struct {
	OperatorLabel string
}

type WorkerConfig struct {
	Enabled                  bool
	BatchSize                int
	StartDelay               time.Duration
	ActiveRunTimeout         time.Duration
	ActiveRunSweepInterval   time.Duration
	ReviewSweepInterval      time.Duration
	ProgressSweepInterval    time.Duration
	CompletionSweepInterval  time.Duration
	PresenceSweepInterval    time.Duration
	PresenceTTL              time.Duration
	TaskSummarySweepInterval time.Duration
	HandoffSweepInterval     time.Duration
	DailySummaryCron         string
}

type RateLimitConfig struct {
	Enabled         bool
	Capacity        int
	RefillPerSecond float64
	KeyPrefix       string
}

func Load() (Config, error) {
	port, err := envInt("AITASK_HTTP_PORT", defaultHTTPPort)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := envDuration("AITASK_HTTP_READ_TIMEOUT", defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := envDuration("AITASK_HTTP_WRITE_TIMEOUT", defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := envDuration("AITASK_HTTP_IDLE_TIMEOUT", defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := envDuration("AITASK_HTTP_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	postgresPort, err := envInt("POSTGRES_PORT", defaultPostgresPort)
	if err != nil {
		return Config{}, err
	}
	dragonflyPort, err := envInt("DRAGONFLY_PORT", defaultDragonflyPort)
	if err != nil {
		return Config{}, err
	}
	workerEnabled, err := envBool("AITASK_WORKER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	workerBatchSize, err := envInt("AITASK_WORKER_BATCH_SIZE", 200)
	if err != nil {
		return Config{}, err
	}
	workerStartDelay, err := envDuration("AITASK_WORKER_START_DELAY", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerActiveRunTimeout, err := envDuration("AITASK_WORKER_ACTIVE_RUN_TIMEOUT", 90*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerActiveRunSweep, err := envDuration("AITASK_WORKER_ACTIVE_RUN_SWEEP_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerReviewSweep, err := envDuration("AITASK_WORKER_REVIEW_SWEEP_INTERVAL", 45*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerProgressSweep, err := envDuration("AITASK_WORKER_PROGRESS_SWEEP_INTERVAL", 45*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerCompletionSweep, err := envDuration("AITASK_WORKER_COMPLETION_SWEEP_INTERVAL", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerPresenceSweep, err := envDuration("AITASK_WORKER_PRESENCE_SWEEP_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerPresenceTTL, err := envDuration("AITASK_WORKER_PRESENCE_TTL", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerTaskSummarySweep, err := envDuration("AITASK_WORKER_TASK_SUMMARY_SWEEP_INTERVAL", 90*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerHandoffSweep, err := envDuration("AITASK_WORKER_HANDOFF_SWEEP_INTERVAL", 90*time.Second)
	if err != nil {
		return Config{}, err
	}
	rateLimitEnabled, err := envBool("AITASK_RATE_LIMIT_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	rateLimitCapacity, err := envInt("AITASK_RATE_LIMIT_CAPACITY", 120)
	if err != nil {
		return Config{}, err
	}
	rateLimitRefill, err := envFloat("AITASK_RATE_LIMIT_REFILL_PER_SECOND", 2.0)
	if err != nil {
		return Config{}, err
	}
	openVikingSettingsKey, err := envBase64AESKey("OPENVIKING_SETTINGS_KEY")
	if err != nil {
		return Config{}, err
	}

	postgresHost := envString("POSTGRES_HOST", defaultPostgresHost)
	postgresDB := envString("POSTGRES_DB", defaultPostgresDB)
	postgresUser := envString("POSTGRES_USER", defaultPostgresUser)
	postgresPassword := envString("POSTGRES_PASSWORD", defaultPostgresPass)
	defaultPostgresURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		postgresUser,
		postgresPassword,
		postgresHost,
		postgresPort,
		postgresDB,
	)

	dragonflyHost := envString("DRAGONFLY_HOST", defaultDragonflyHost)
	dragonflyPassword := envString("DRAGONFLY_PASSWORD", defaultDragonflyPass)
	defaultDragonflyURL := fmt.Sprintf(
		"redis://:%s@%s:%d/0",
		dragonflyPassword,
		dragonflyHost,
		dragonflyPort,
	)

	cfg := Config{
		Environment: envString("AITASK_ENV", defaultEnvironment),
		Service: ServiceConfig{
			Name:    envString("AITASK_SERVICE_NAME", defaultServiceName),
			Version: envString("AITASK_SERVICE_VERSION", defaultServiceVersion),
		},
		Server: ServerConfig{
			Host:            envString("AITASK_HTTP_HOST", defaultHTTPHost),
			Port:            port,
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			ShutdownTimeout: shutdownTimeout,
		},
		Postgres: PostgresConfig{
			Host:     postgresHost,
			Port:     postgresPort,
			DB:       postgresDB,
			User:     postgresUser,
			Password: postgresPassword,
			URL:      envString("DATABASE_URL", defaultPostgresURL),
		},
		Dragonfly: DragonflyConfig{
			Host:     dragonflyHost,
			Port:     dragonflyPort,
			Password: dragonflyPassword,
			URL:      envString("DRAGONFLY_URL", defaultDragonflyURL),
		},
		OpenViking: OpenVikingConfig{
			Namespace:   envString("OPENVIKING_NAMESPACE", defaultOpenVikingNS),
			SettingsKey: openVikingSettingsKey,
		},
		Security: SecurityConfig{
			AgentTokenSecret: envString("AGENT_TOKEN_SECRET", defaultAgentSecret),
		},
		Console: ConsoleConfig{
			OperatorLabel: envString("CONSOLE_OPERATOR_LABEL", defaultOperatorLabel),
		},
		Worker: WorkerConfig{
			Enabled:                  workerEnabled,
			BatchSize:                workerBatchSize,
			StartDelay:               workerStartDelay,
			ActiveRunTimeout:         workerActiveRunTimeout,
			ActiveRunSweepInterval:   workerActiveRunSweep,
			ReviewSweepInterval:      workerReviewSweep,
			ProgressSweepInterval:    workerProgressSweep,
			CompletionSweepInterval:  workerCompletionSweep,
			PresenceSweepInterval:    workerPresenceSweep,
			PresenceTTL:              workerPresenceTTL,
			TaskSummarySweepInterval: workerTaskSummarySweep,
			HandoffSweepInterval:     workerHandoffSweep,
			DailySummaryCron:         envString("AITASK_WORKER_DAILY_SUMMARY_CRON", defaultWorkerCron),
		},
		RateLimit: RateLimitConfig{
			Enabled:         rateLimitEnabled,
			Capacity:        rateLimitCapacity,
			RefillPerSecond: rateLimitRefill,
			KeyPrefix:       envString("AITASK_RATE_LIMIT_KEY_PREFIX", "rl:token"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.Environment), "production")
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Service.Name) == "" {
		return fmt.Errorf("AITASK_SERVICE_NAME cannot be empty")
	}
	if err := c.Server.Validate(); err != nil {
		return err
	}
	if err := c.Postgres.Validate(); err != nil {
		return err
	}
	if err := c.Dragonfly.Validate(); err != nil {
		return err
	}
	if err := c.OpenViking.Validate(); err != nil {
		return err
	}
	if err := c.Security.Validate(); err != nil {
		return err
	}
	if err := c.Console.Validate(); err != nil {
		return err
	}
	if err := c.Worker.Validate(); err != nil {
		return err
	}
	if err := c.RateLimit.Validate(); err != nil {
		return err
	}
	return nil
}

func (c ServerConfig) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func (c ServerConfig) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("AITASK_HTTP_HOST cannot be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("AITASK_HTTP_PORT must be between 1 and 65535")
	}
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("AITASK_HTTP_READ_TIMEOUT must be positive")
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("AITASK_HTTP_WRITE_TIMEOUT must be positive")
	}
	if c.IdleTimeout <= 0 {
		return fmt.Errorf("AITASK_HTTP_IDLE_TIMEOUT must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("AITASK_HTTP_SHUTDOWN_TIMEOUT must be positive")
	}
	return nil
}

func (c PostgresConfig) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("POSTGRES_HOST cannot be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("POSTGRES_PORT must be between 1 and 65535")
	}
	if strings.TrimSpace(c.DB) == "" {
		return fmt.Errorf("POSTGRES_DB cannot be empty")
	}
	if strings.TrimSpace(c.User) == "" {
		return fmt.Errorf("POSTGRES_USER cannot be empty")
	}
	if strings.TrimSpace(c.Password) == "" {
		return fmt.Errorf("POSTGRES_PASSWORD cannot be empty")
	}
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("DATABASE_URL cannot be empty")
	}
	return nil
}

func (c DragonflyConfig) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("DRAGONFLY_HOST cannot be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("DRAGONFLY_PORT must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Password) == "" {
		return fmt.Errorf("DRAGONFLY_PASSWORD cannot be empty")
	}
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("DRAGONFLY_URL cannot be empty")
	}
	return nil
}

func (c OpenVikingConfig) Validate() error {
	if strings.TrimSpace(c.Namespace) == "" {
		return fmt.Errorf("OPENVIKING_NAMESPACE cannot be empty")
	}
	if len(c.SettingsKey) != 32 {
		return fmt.Errorf("OPENVIKING_SETTINGS_KEY must decode to 32 bytes")
	}
	return nil
}

func (c SecurityConfig) Validate() error {
	if strings.TrimSpace(c.AgentTokenSecret) == "" {
		return fmt.Errorf("AGENT_TOKEN_SECRET cannot be empty")
	}
	return nil
}

func (c ConsoleConfig) Validate() error {
	if strings.TrimSpace(c.OperatorLabel) == "" {
		return fmt.Errorf("CONSOLE_OPERATOR_LABEL cannot be empty")
	}
	return nil
}

func (c WorkerConfig) Validate() error {
	if c.BatchSize <= 0 {
		return fmt.Errorf("AITASK_WORKER_BATCH_SIZE must be positive")
	}
	if c.StartDelay < 0 {
		return fmt.Errorf("AITASK_WORKER_START_DELAY cannot be negative")
	}
	if c.ActiveRunTimeout <= 0 {
		return fmt.Errorf("AITASK_WORKER_ACTIVE_RUN_TIMEOUT must be positive")
	}
	if c.ActiveRunSweepInterval <= 0 {
		return fmt.Errorf("AITASK_WORKER_ACTIVE_RUN_SWEEP_INTERVAL must be positive")
	}
	if c.ReviewSweepInterval <= 0 {
		return fmt.Errorf("AITASK_WORKER_REVIEW_SWEEP_INTERVAL must be positive")
	}
	if c.ProgressSweepInterval <= 0 {
		return fmt.Errorf("AITASK_WORKER_PROGRESS_SWEEP_INTERVAL must be positive")
	}
	if c.CompletionSweepInterval <= 0 {
		return fmt.Errorf("AITASK_WORKER_COMPLETION_SWEEP_INTERVAL must be positive")
	}
	if c.PresenceSweepInterval <= 0 {
		return fmt.Errorf("AITASK_WORKER_PRESENCE_SWEEP_INTERVAL must be positive")
	}
	if c.PresenceTTL <= 0 {
		return fmt.Errorf("AITASK_WORKER_PRESENCE_TTL must be positive")
	}
	if c.TaskSummarySweepInterval <= 0 {
		return fmt.Errorf("AITASK_WORKER_TASK_SUMMARY_SWEEP_INTERVAL must be positive")
	}
	if c.HandoffSweepInterval <= 0 {
		return fmt.Errorf("AITASK_WORKER_HANDOFF_SWEEP_INTERVAL must be positive")
	}
	if strings.TrimSpace(c.DailySummaryCron) == "" {
		return fmt.Errorf("AITASK_WORKER_DAILY_SUMMARY_CRON cannot be empty")
	}
	return nil
}

func (c RateLimitConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Capacity <= 0 {
		return fmt.Errorf("AITASK_RATE_LIMIT_CAPACITY must be positive")
	}
	if c.RefillPerSecond <= 0 {
		return fmt.Errorf("AITASK_RATE_LIMIT_REFILL_PER_SECOND must be positive")
	}
	if strings.TrimSpace(c.KeyPrefix) == "" {
		return fmt.Errorf("AITASK_RATE_LIMIT_KEY_PREFIX cannot be empty")
	}
	return nil
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return parsed, nil
}

func envBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}

func envBase64AESKey(key string) ([]byte, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		generated := make([]byte, 32)
		if _, err := rand.Read(generated); err != nil {
			return nil, fmt.Errorf("%s random fallback failed: %w", key, err)
		}
		slog.Warn("OPENVIKING_SETTINGS_KEY is not set; using random process-local key, stored project OpenViking API keys will not survive restart")
		return generated, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64 encoded: %w", key, err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("%s must decode to 32 bytes", key)
	}
	return decoded, nil
}

func envFloat(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a float: %w", key, err)
	}
	return parsed, nil
}
