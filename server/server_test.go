// server/server_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func setupTestServer(t *testing.T) *Server {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")

	cfg := &config.Config{
		Server:  config.ServerConfig{Host: "localhost", Port: 8080},
		JWT:     config.JWTConfig{Secret: "test-secret-32-chars-minimum!!"},
		Session: config.SessionConfig{},
	}

	return New(cfg, coreDB)
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

func TestServer_Login_Success(t *testing.T) {
	srv := setupTestServer(t)

	// Create user
	hash, _ := auth.HashPassword("testpass")
	srv.db.CreateUser("testuser", hash)

	body := `{"username":"testuser","password":"testpass"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["access_token"] == "" {
		t.Error("expected access_token in response")
	}
	if resp["refresh_token"] == "" {
		t.Error("expected refresh_token in response")
	}
}

func TestServer_Login_InvalidCredentials(t *testing.T) {
	srv := setupTestServer(t)

	hash, _ := auth.HashPassword("testpass")
	srv.db.CreateUser("testuser", hash)

	body := `{"username":"testuser","password":"wrongpass"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestServer_Login_UserNotFound(t *testing.T) {
	srv := setupTestServer(t)

	body := `{"username":"nouser","password":"pass"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestServer_Refresh_Success(t *testing.T) {
	srv := setupTestServer(t)

	// Create user and token
	hash, _ := auth.HashPassword("testpass")
	user, _ := srv.db.CreateUser("testuser", hash)
	srv.db.CreateRefreshToken(user.ID, "valid-refresh-token", time.Now().Add(24*time.Hour))

	body := `{"refresh_token":"valid-refresh-token"}`
	req := httptest.NewRequest("POST", "/api/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["access_token"] == "" {
		t.Error("expected new access_token")
	}
}

func TestServer_Refresh_InvalidToken(t *testing.T) {
	srv := setupTestServer(t)

	body := `{"refresh_token":"invalid-token"}`
	req := httptest.NewRequest("POST", "/api/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestServer_Refresh_ExpiredToken(t *testing.T) {
	srv := setupTestServer(t)

	hash, _ := auth.HashPassword("testpass")
	user, _ := srv.db.CreateUser("testuser", hash)
	srv.db.CreateRefreshToken(user.ID, "expired-token", time.Now().Add(-1*time.Hour))

	body := `{"refresh_token":"expired-token"}`
	req := httptest.NewRequest("POST", "/api/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestServer_Logout(t *testing.T) {
	srv := setupTestServer(t)

	hash, _ := auth.HashPassword("testpass")
	user, _ := srv.db.CreateUser("testuser", hash)
	srv.db.CreateRefreshToken(user.ID, "logout-token", time.Now().Add(24*time.Hour))

	body := `{"refresh_token":"logout-token"}`
	req := httptest.NewRequest("POST", "/api/logout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Token should be deleted
	_, err := srv.db.GetRefreshToken("logout-token")
	if err != db.ErrNotFound {
		t.Error("expected token to be deleted")
	}
}

func TestMessageEndpointsRequireAuth(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	cfg := &config.Config{
		JWT:     config.JWTConfig{Secret: "testsecret"},
		Session: config.SessionConfig{},
		History: config.HistoryConfig{
			DefaultLimit: 50,
			MaxLimit:     100,
		},
		Sync: config.SyncConfig{
			MaxLookback: 24 * time.Hour,
		},
	}
	srv := New(cfg, coreDB)

	endpoints := []string{
		"/api/messages/recent",
		"/api/messages/history?before=1",
		"/api/messages/sync?since=2020-01-01T00:00:00Z",
	}

	for _, endpoint := range endpoints {
		req := httptest.NewRequest("GET", endpoint, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", endpoint, rec.Code)
		}
	}
}

func TestLogin_BlockedUser(t *testing.T) {
	srv := setupTestServer(t)

	// Create and block a user
	hash, _ := auth.HashPassword("password")
	user, _ := srv.db.CreateUserFull("blocked", hash, "Blocked User", "user")
	srv.db.BlockUser(user.ID)

	// Try to login
	body := `{"username":"blocked","password":"password"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRefresh_BlockedUser(t *testing.T) {
	srv := setupTestServer(t)

	// Create user and get tokens
	hash, _ := auth.HashPassword("password")
	user, _ := srv.db.CreateUserFull("toblock", hash, "To Block", "user")

	// Create refresh token
	srv.db.CreateRefreshToken(user.ID, "block-test-token", time.Now().Add(24*time.Hour))

	// Block the user
	srv.db.BlockUser(user.ID)

	// Try to refresh
	body := `{"refresh_token":"block-test-token"}`
	req := httptest.NewRequest("POST", "/api/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSignup_ValidInvite(t *testing.T) {
	srv := setupTestServer(t)

	// Create admin and invite
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	srv.db.CreateInvite(admin.ID, "validcode")

	body := `{"code":"validcode","username":"newuser","display_name":"New User","password":"password123"}`
	req := httptest.NewRequest("POST", "/api/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify user was created
	user, err := srv.db.GetUserByUsername("newuser")
	if err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.DisplayName != "New User" {
		t.Errorf("expected display name 'New User', got %s", user.DisplayName)
	}
	if user.Role != "user" {
		t.Errorf("expected role 'user', got %s", user.Role)
	}

	// Verify invite was marked as used
	invite, _ := srv.db.GetInviteByCode("validcode")
	if invite.UsedBy == nil {
		t.Error("invite should be marked as used")
	}
}

func TestSignup_InvalidInvite(t *testing.T) {
	srv := setupTestServer(t)

	body := `{"code":"invalidcode","username":"newuser","display_name":"New User","password":"password123"}`
	req := httptest.NewRequest("POST", "/api/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSignup_UsernameValidation(t *testing.T) {
	srv := setupTestServer(t)

	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	srv.db.CreateInvite(admin.ID, "testcode")

	// Too short
	body := `{"code":"testcode","username":"ab","display_name":"Test","password":"password123"}`
	req := httptest.NewRequest("POST", "/api/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short username, got %d", w.Code)
	}
}

func TestSignup_PasswordValidation(t *testing.T) {
	srv := setupTestServer(t)

	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	srv.db.CreateInvite(admin.ID, "testcode2")

	// Too short
	body := `{"code":"testcode2","username":"validuser","display_name":"Test","password":"short"}`
	req := httptest.NewRequest("POST", "/api/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", w.Code)
	}
}

func TestSignup_UsedInvite(t *testing.T) {
	srv := setupTestServer(t)

	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	srv.db.CreateInvite(admin.ID, "usedcode")

	// Use the invite first
	user, _ := srv.db.CreateUserFull("firstuser", "hash", "First", "user")
	srv.db.UseInvite("usedcode", user.ID)

	// Try to use it again
	body := `{"code":"usedcode","username":"seconduser","display_name":"Second","password":"password123"}`
	req := httptest.NewRequest("POST", "/api/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for used invite, got %d", w.Code)
	}
}
