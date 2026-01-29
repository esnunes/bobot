// server/server_test.go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func setupTestServer(t *testing.T) *Server {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")
	jwtSvc := auth.NewJWTService("test-secret-32-chars-minimum!!")

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: 8080},
	}

	return New(cfg, coreDB, jwtSvc)
}

func TestServer_HealthCheck(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
