// config/config_test.go
package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_RequiredFields(t *testing.T) {
	// Clear env
	os.Clearenv()

	// Set required fields
	os.Setenv("BOBOT_LLM_BASE_URL", "https://api.z.ai")
	os.Setenv("BOBOT_LLM_API_KEY", "test-key")
	os.Setenv("BOBOT_LLM_MODEL", "glm-4.7")
	os.Setenv("BOBOT_JWT_SECRET", "test-secret-32-chars-minimum!!")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LLM.BaseURL != "https://api.z.ai" {
		t.Errorf("expected BaseURL https://api.z.ai, got %s", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "test-key" {
		t.Errorf("expected APIKey test-key, got %s", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "glm-4.7" {
		t.Errorf("expected Model glm-4.7, got %s", cfg.LLM.Model)
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("BOBOT_LLM_BASE_URL", "https://api.z.ai")
	os.Setenv("BOBOT_LLM_API_KEY", "test-key")
	os.Setenv("BOBOT_LLM_MODEL", "glm-4.7")
	os.Setenv("BOBOT_JWT_SECRET", "test-secret-32-chars-minimum!!")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("expected default data dir ./data, got %s", cfg.DataDir)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Clearenv()

	_, err := Load()
	if err == nil {
		t.Error("expected error for missing required fields")
	}
}

func TestLoad_ContextConfig_Defaults(t *testing.T) {
	// Set required env vars
	os.Setenv("BOBOT_LLM_BASE_URL", "http://test")
	os.Setenv("BOBOT_LLM_API_KEY", "key")
	os.Setenv("BOBOT_LLM_MODEL", "model")
	os.Setenv("BOBOT_JWT_SECRET", "secret")
	defer func() {
		os.Unsetenv("BOBOT_LLM_BASE_URL")
		os.Unsetenv("BOBOT_LLM_API_KEY")
		os.Unsetenv("BOBOT_LLM_MODEL")
		os.Unsetenv("BOBOT_JWT_SECRET")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Context.TokensStart != 30000 {
		t.Errorf("expected TokensStart 30000, got %d", cfg.Context.TokensStart)
	}
	if cfg.Context.TokensMax != 80000 {
		t.Errorf("expected TokensMax 80000, got %d", cfg.Context.TokensMax)
	}
	if cfg.History.DefaultLimit != 50 {
		t.Errorf("expected DefaultLimit 50, got %d", cfg.History.DefaultLimit)
	}
	if cfg.History.MaxLimit != 100 {
		t.Errorf("expected MaxLimit 100, got %d", cfg.History.MaxLimit)
	}
	if cfg.Sync.MaxLookback != 24*time.Hour {
		t.Errorf("expected MaxLookback 24h, got %v", cfg.Sync.MaxLookback)
	}
}

func TestSessionConfigDefaults(t *testing.T) {
	// Set required env vars
	os.Setenv("BOBOT_LLM_BASE_URL", "http://test")
	os.Setenv("BOBOT_LLM_API_KEY", "test-key")
	os.Setenv("BOBOT_LLM_MODEL", "test-model")
	os.Setenv("BOBOT_JWT_SECRET", "test-secret-key")
	defer func() {
		os.Unsetenv("BOBOT_LLM_BASE_URL")
		os.Unsetenv("BOBOT_LLM_API_KEY")
		os.Unsetenv("BOBOT_LLM_MODEL")
		os.Unsetenv("BOBOT_JWT_SECRET")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Session.Duration != 30*time.Minute {
		t.Errorf("Session.Duration = %v, want 30m", cfg.Session.Duration)
	}
	if cfg.Session.MaxAge != 7*24*time.Hour {
		t.Errorf("Session.MaxAge = %v, want 168h", cfg.Session.MaxAge)
	}
	if cfg.Session.RefreshThreshold != 5*time.Minute {
		t.Errorf("Session.RefreshThreshold = %v, want 5m", cfg.Session.RefreshThreshold)
	}
}
