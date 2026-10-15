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
}

// Load loads configuration from environment variables with sensible defaults.
func Load() *Config {
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
