// config/config_test.go
package config

import (
	"os"
	"testing"
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
