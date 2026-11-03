package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config stores persistent CLI preferences and credentials.
type Config struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
	JSON     bool   `json:"json,omitempty"`
}

// ConfigPath returns the path to the CLI configuration file (~/.kerob/config.json).
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kerob.json"
	}
	return filepath.Join(home, ".kerob", "config.json")
}

// LoadConfig loads the CLI configuration from disk or defaults.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Endpoint: "http://localhost:8080",
	}

	// Environment variable overrides take precedence
	if envEndpoint := os.Getenv("KEROB_ENDPOINT"); envEndpoint != "" {
		cfg.Endpoint = envEndpoint
	}
	if envToken := os.Getenv("KEROB_TOKEN"); envToken != "" {
		cfg.Token = envToken
	}
	if envTenant := os.Getenv("KEROB_TENANT_ID"); envTenant != "" {
		cfg.TenantID = envTenant
	}

	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, nil
	}

	var diskCfg Config
	if err := json.Unmarshal(data, &diskCfg); err == nil {
		if cfg.Endpoint == "http://localhost:8080" && diskCfg.Endpoint != "" {
			cfg.Endpoint = diskCfg.Endpoint
		}
		if cfg.Token == "" && diskCfg.Token != "" {
			cfg.Token = diskCfg.Token
		}
		if cfg.TenantID == "" && diskCfg.TenantID != "" {
			cfg.TenantID = diskCfg.TenantID
		}
		cfg.JSON = diskCfg.JSON
	}

	return cfg, nil
}

// SaveConfig persists the CLI configuration to ~/.kerob/config.json with restrictive permissions (0600).
func SaveConfig(cfg *Config) error {
	path := ConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %q: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config to %q: %w", path, err)
	}
	return nil
}
