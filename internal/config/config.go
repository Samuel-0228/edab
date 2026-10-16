package config

import (
	"os"
	"strconv"
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

	// Telegram integration
	TelegramBotToken       string
	TelegramEnabled        bool
	TelegramAPIBaseURL     string
	TelegramPollTimeout    int
	TelegramMediaGroupWait time.Duration
	TelegramRateGlobal         float64
	TelegramRatePerChat        float64
	TelegramPublicPollInterval time.Duration
}

// Load loads configuration from environment variables with sensible defaults.
func Load() *Config {
	botToken := getEnv("TELEGRAM_BOT_TOKEN", "")
	return &Config{
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/automation?sslmode=disable"),
		ServerPort:         getEnv("PORT", getEnv("SERVER_PORT", "8080")),
		WorkerPollInterval: getEnvDuration("WORKER_POLL_INTERVAL", 1*time.Second),
		WorkerConcurrency:  getEnvInt("WORKER_CONCURRENCY", 4),
		MaxRetries:         getEnvInt("MAX_RETRIES", 3),
		InitialBackoff:     getEnvDuration("INITIAL_BACKOFF", 2*time.Second),
		MaxBackoff:         getEnvDuration("MAX_BACKOFF", 1*time.Minute),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		Environment:        getEnv("ENVIRONMENT", "development"),

		TelegramBotToken:           botToken,
		TelegramEnabled:            getEnvBool("TELEGRAM_ENABLED", botToken != ""),
		TelegramAPIBaseURL:         getEnv("TELEGRAM_API_BASE_URL", ""),
		TelegramPollTimeout:        getEnvInt("TELEGRAM_POLL_TIMEOUT", 30),
		TelegramMediaGroupWait:     getEnvDuration("TELEGRAM_MEDIA_GROUP_WINDOW", 1*time.Second),
		TelegramRateGlobal:         getEnvFloat("TELEGRAM_RATE_GLOBAL", 25),
		TelegramRatePerChat:        getEnvFloat("TELEGRAM_RATE_PER_CHAT", 1),
		TelegramPublicPollInterval: getEnvDuration("TELEGRAM_PUBLIC_POLL_INTERVAL", 15*time.Second),
	}
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

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}
