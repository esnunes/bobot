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
	Session  SessionConfig
	Context  ContextConfig
	History  HistoryConfig
	Sync     SyncConfig
	VAPID    VAPIDConfig
	Schedule ScheduleConfig
	DataDir            string
	BaseURL            string
	BraveSearchAPIKey  string
	GoogleClientID     string
	GoogleClientSecret string
	SpotifyClientID     string
	SpotifyClientSecret string
}

type VAPIDConfig struct {
	PublicKey  string // base64url-encoded 65-byte uncompressed P-256 public key
	PrivateKey string // base64url-encoded 32-byte raw private key scalar
	Subject    string // mailto: or https: URL
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

type SessionConfig struct {
	Duration         time.Duration
	MaxAge           time.Duration
	RefreshThreshold time.Duration
}

type ScheduleConfig struct {
	Timeout time.Duration // BOBOT_SCHEDULE_TIMEOUT, default 5m
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
		Session: SessionConfig{
			Duration:         getEnvDurationOrDefault("BOBOT_SESSION_DURATION", 30*time.Minute),
			MaxAge:           getEnvDurationOrDefault("BOBOT_SESSION_MAX_AGE", 7*24*time.Hour),
			RefreshThreshold: getEnvDurationOrDefault("BOBOT_SESSION_REFRESH_THRESHOLD", 5*time.Minute),
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
		Schedule: ScheduleConfig{
			Timeout: getEnvDurationOrDefault("BOBOT_SCHEDULE_TIMEOUT", 5*time.Minute),
		},
		VAPID: VAPIDConfig{
			PublicKey:  os.Getenv("BOBOT_VAPID_PUBLIC_KEY"),
			PrivateKey: os.Getenv("BOBOT_VAPID_PRIVATE_KEY"),
			Subject:    os.Getenv("BOBOT_VAPID_SUBJECT"),
		},
		DataDir:            getEnvOrDefault("BOBOT_DATA_DIR", "./data"),
		BaseURL:            getEnvOrDefault("BOBOT_BASE_URL", "http://localhost:8080"),
		BraveSearchAPIKey:  os.Getenv("BRAVE_SEARCH_API_KEY"),
		GoogleClientID:     os.Getenv("BOBOT_GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("BOBOT_GOOGLE_CLIENT_SECRET"),
		SpotifyClientID:     os.Getenv("BOBOT_SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("BOBOT_SPOTIFY_CLIENT_SECRET"),
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
