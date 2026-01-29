// config/config.go
package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Server   ServerConfig
	LLM      LLMConfig
	JWT      JWTConfig
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
