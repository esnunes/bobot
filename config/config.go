// config/config.go
package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	LLM      LLMConfig
	JWT      JWTConfig
	Context  ContextConfig
	History  HistoryConfig
	Sync     SyncConfig
	DataDir  string
	InitUser string
	InitPass string
}

type ServerConfig struct {
	Host string
	Port int
}

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type JWTConfig struct {
	Secret string
}

type ContextConfig struct {
	TokensStart int
	TokensMax   int
}

type HistoryConfig struct {
	DefaultLimit int
	MaxLimit     int
}

type SyncConfig struct {
	MaxLookback time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: getEnvOrDefault("BOBOT_HOST", "0.0.0.0"),
			Port: getEnvIntOrDefault("BOBOT_PORT", 8080),
		},
		LLM: LLMConfig{
			BaseURL: os.Getenv("BOBOT_LLM_BASE_URL"),
			APIKey:  os.Getenv("BOBOT_LLM_API_KEY"),
			Model:   os.Getenv("BOBOT_LLM_MODEL"),
		},
		JWT: JWTConfig{
			Secret: os.Getenv("BOBOT_JWT_SECRET"),
		},
		Context: ContextConfig{
			TokensStart: getEnvIntOrDefault("BOBOT_CONTEXT_TOKENS_START", 30000),
			TokensMax:   getEnvIntOrDefault("BOBOT_CONTEXT_TOKENS_MAX", 80000),
		},
		History: HistoryConfig{
			DefaultLimit: getEnvIntOrDefault("BOBOT_HISTORY_DEFAULT_LIMIT", 50),
			MaxLimit:     getEnvIntOrDefault("BOBOT_HISTORY_MAX_LIMIT", 100),
		},
		Sync: SyncConfig{
			MaxLookback: getEnvDurationOrDefault("BOBOT_SYNC_MAX_LOOKBACK", 24*time.Hour),
		},
		DataDir:  getEnvOrDefault("BOBOT_DATA_DIR", "./data"),
		InitUser: os.Getenv("BOBOT_INIT_USER"),
		InitPass: os.Getenv("BOBOT_INIT_PASS"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.LLM.BaseURL == "" {
		return errors.New("BOBOT_LLM_BASE_URL is required")
	}
	if c.LLM.APIKey == "" {
		return errors.New("BOBOT_LLM_API_KEY is required")
	}
	if c.LLM.Model == "" {
		return errors.New("BOBOT_LLM_MODEL is required")
	}
	if c.JWT.Secret == "" {
		return errors.New("BOBOT_JWT_SECRET is required")
	}
	return nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvIntOrDefault(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
