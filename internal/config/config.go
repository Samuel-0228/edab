package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all engine configuration loaded from environment variables.
type Config struct {
	DatabaseURL        string
	ServerPort         string
	WorkerPollInterval time.Duration
	WorkerConcurrency  int
	MaxRetries         int
	InitialBackoff     time.Duration
	MaxBackoff         time.Duration
	LogLevel           string
	Environment        string
	APIToken           string
	EncryptionKey      string
	SchedulerInterval  time.Duration

	TelegramBotToken           string
	TelegramEnabled            bool
	TelegramAPIBaseURL         string
	TelegramPollTimeout        int
	TelegramMediaGroupWait     time.Duration
	TelegramRateGlobal         float64
	TelegramRatePerChat        float64
	TelegramPublicPollInterval time.Duration
}

// Load loads configuration from environment variables with sensible defaults.
func Load() *Config {
	botToken := getEnv("TELEGRAM_BOT_TOKEN", "")
	return &Config{
		DatabaseURL:                getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/automation?sslmode=disable"),
		ServerPort:                 getEnv("PORT", getEnv("SERVER_PORT", "8080")),
		WorkerPollInterval:         getEnvDuration("WORKER_POLL_INTERVAL", time.Second),
		WorkerConcurrency:          getEnvInt("WORKER_CONCURRENCY", 4),
		MaxRetries:                 getEnvInt("MAX_RETRIES", 3),
		InitialBackoff:             getEnvDuration("INITIAL_BACKOFF", 2*time.Second),
		MaxBackoff:                 getEnvDuration("MAX_BACKOFF", time.Minute),
		LogLevel:                   getEnv("LOG_LEVEL", "info"),
		Environment:                getEnv("ENVIRONMENT", "development"),
		APIToken:                   getEnv("API_TOKEN", ""),
		EncryptionKey:              getEnv("KEROB_ENCRYPTION_KEY", getEnv("ENCRYPTION_KEY", "kerob-default-local-encryption-key-32bytes!!")),
		SchedulerInterval:          getEnvDuration("SCHEDULER_INTERVAL", 10*time.Second),
		TelegramBotToken:           botToken,
		TelegramEnabled:            getEnvBool("TELEGRAM_ENABLED", botToken != ""),
		TelegramAPIBaseURL:         getEnv("TELEGRAM_API_BASE_URL", ""),
		TelegramPollTimeout:        getEnvInt("TELEGRAM_POLL_TIMEOUT", 30),
		TelegramMediaGroupWait:     getEnvDuration("TELEGRAM_MEDIA_GROUP_WINDOW", time.Second),
		TelegramRateGlobal:         getEnvFloat("TELEGRAM_RATE_GLOBAL", 25),
		TelegramRatePerChat:        getEnvFloat("TELEGRAM_RATE_PER_CHAT", 1),
		TelegramPublicPollInterval: getEnvDuration("TELEGRAM_PUBLIC_POLL_INTERVAL", 15*time.Second),
	}
}

// IsProduction reports whether the engine is configured to run in production mode.
func (c *Config) IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	return env == "production" || env == "prod"
}

// IsDevelopment reports whether the engine is configured to run in development mode.
func (c *Config) IsDevelopment() bool {
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	return env == "development" || env == "dev" || env == "" || env == "local"
}

var knownInsecureTokens = map[string]bool{
	"your-random-production-api-token-here":             true,
	"your-secure-admin-token":                           true,
	"replace-with-a-secure-random-32-byte-secret-key!!": true,
	"changeme": true,
	"admin":    true,
	"password": true,
	"secret":   true,
	"12345678": true,
	"token":    true,
	"default":  true,
}

func isPlaceholderToken(token string) bool {
	return knownInsecureTokens[strings.ToLower(strings.TrimSpace(token))]
}

// Validate checks settings that would otherwise allow an unprotected
// management API in a production deployment or invalid runtime constraints.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.IsProduction() {
		token := strings.TrimSpace(c.APIToken)
		if token == "" {
			return fmt.Errorf("API_TOKEN is required in production")
		}
		if isPlaceholderToken(token) {
			return fmt.Errorf("API_TOKEN cannot be a placeholder or default value in production")
		}
		if len(token) < 16 {
			return fmt.Errorf("API_TOKEN must be at least 16 characters in production")
		}
	}
	if c.WorkerConcurrency < 1 {
		return fmt.Errorf("WORKER_CONCURRENCY must be at least 1")
	}
	if c.MaxRetries < 1 {
		return fmt.Errorf("MAX_RETRIES must be at least 1")
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
