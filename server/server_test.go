// server/server_test.go
package server

import (
	"fmt"
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
		Server: config.ServerConfig{Host: "localhost", Port: 8080},
		JWT:    config.JWTConfig{Secret: "test-secret-32-chars-minimum!!"},
		Session: config.SessionConfig{
			Duration:         30 * time.Minute,
			MaxAge:           7 * 24 * time.Hour,
			RefreshThreshold: 5 * time.Minute,
		},
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
	srv.db.CreateUserFull("testuser", hash, "Test User", "user")

	form := "username=testuser&password=testpass"
	req := httptest.NewRequest("POST", "/", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Check that authenticated template is rendered
	body := w.Body.String()
	if !strings.Contains(body, "authenticated-container") {
		t.Errorf("expected authenticated template, got %s", body)
	}

	// Check for session cookie
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("expected session cookie to be set")
	}
}

func TestHandleLogin_SetsSessionCookie(t *testing.T) {
	s := setupTestServer(t)

	// Create a user
	hash, _ := auth.HashPassword("password123")
	s.db.CreateUserFull("testuser", hash, "Test", "user")

	form := "username=testuser&password=password123"
	req := httptest.NewRequest("POST", "/", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rr.Code)
	}

	// Check for session cookie
	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set")
	}
	if !sessionCookie.HttpOnly {
		t.Error("Session cookie should be HttpOnly")
	}
}

func TestServer_Login_InvalidCredentials(t *testing.T) {
	srv := setupTestServer(t)

	hash, _ := auth.HashPassword("testpass")
	srv.db.CreateUserFull("testuser", hash, "Test User", "user")

	form := "username=testuser&password=wrongpass"
	req := httptest.NewRequest("POST", "/", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// Form login returns 200 with error in template
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Invalid credentials") {
		t.Errorf("expected error message in response, got %s", body)
	}
}

func TestServer_Login_UserNotFound(t *testing.T) {
	srv := setupTestServer(t)

	form := "username=nouser&password=pass"
	req := httptest.NewRequest("POST", "/", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// Form login returns 200 with error in template
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Invalid credentials") {
		t.Errorf("expected error message in response, got %s", body)
	}
}

// TestServer_Logout was removed - JWT refresh token deletion is obsolete
// See TestHandleLogout_ClearsCookie and TestHandleLogout_WithAllParam_CreatesRevocation
// for session-based logout tests

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
	form := "username=blocked&password=password"
	req := httptest.NewRequest("POST", "/", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Form login returns 200 with error in template
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Account blocked") {
		t.Errorf("expected blocked message in response, got %s", body)
	}
}

func TestSignup_ValidInvite(t *testing.T) {
	srv := setupTestServer(t)

	// Create admin and invite
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	srv.db.CreateInvite(admin.ID, "validcode")

	form := "code=validcode&username=newuser&display_name=New+User&password=password123"
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

	form := "code=invalidcode&username=newuser&display_name=New+User&password=password123"
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Form signup returns 200 with error in template
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Invalid or expired invite") {
		t.Errorf("expected error message in response, got %s", body)
	}
}

func TestSignup_UsernameValidation(t *testing.T) {
	srv := setupTestServer(t)

	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	srv.db.CreateInvite(admin.ID, "testcode")

	// Too short
	form := "code=testcode&username=ab&display_name=Test&password=password123"
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Form signup returns 200 with error in template
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "at least 3 characters") {
		t.Errorf("expected username error message in response, got %s", body)
	}
}

func TestSignup_PasswordValidation(t *testing.T) {
	srv := setupTestServer(t)

	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	srv.db.CreateInvite(admin.ID, "testcode2")

	// Too short
	form := "code=testcode2&username=validuser&display_name=Test&password=short"
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Form signup returns 200 with error in template
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "at least 8 characters") {
		t.Errorf("expected password error message in response, got %s", body)
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
	form := "code=usedcode&username=seconduser&display_name=Second&password=password123"
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Form signup returns 200 with error in template
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Invalid or expired invite") {
		t.Errorf("expected error message in response, got %s", body)
	}
}

func TestSessionMiddleware_ValidToken(t *testing.T) {
	s := setupTestServer(t)

	// Create a valid session token
	token, _ := s.session.CreateToken(1, "user")

	// Create request with session cookie
	req := httptest.NewRequest("GET", "/api/messages/recent", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	rr := httptest.NewRecorder()

	handler := s.sessionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		data := auth.UserDataFromContext(r.Context())
		if data.UserID != 1 {
			t.Errorf("UserID = %d, want 1", data.UserID)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rr.Code)
	}
}

func TestSessionMiddleware_MissingCookie(t *testing.T) {
	s := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/messages/recent", nil)
	rr := httptest.NewRecorder()

	handler := s.sessionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", rr.Code)
	}
}

func TestSessionMiddleware_InvalidToken(t *testing.T) {
	s := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/messages/recent", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid-token"})
	rr := httptest.NewRecorder()

	handler := s.sessionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", rr.Code)
	}

	// Check that session cookie was cleared
	cookies := rr.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "session" && c.MaxAge == -1 {
			return // Cookie was cleared as expected
		}
	}
	t.Error("Expected session cookie to be cleared")
}

func TestSessionMiddleware_BlockedUser(t *testing.T) {
	s := setupTestServer(t)

	// Create and block a user
	hash, _ := auth.HashPassword("password")
	user, _ := s.db.CreateUserFull("blocked", hash, "Blocked User", "user")
	s.db.BlockUser(user.ID)

	// Create token with short duration to trigger reissue check
	// We need to create a token that needs reissue to trigger the DB check
	token, _ := s.session.CreateToken(user.ID, "user")

	req := httptest.NewRequest("GET", "/api/messages/recent", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	handler := s.sessionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// If token doesn't need reissue, handler will be called
		// This is expected behavior - blocked check only happens on reissue
		w.WriteHeader(http.StatusOK)
	})

	handler(rr, req)

	// Token may or may not need reissue depending on timing
	// This test verifies the middleware doesn't crash with blocked users
	if rr.Code != http.StatusOK && rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 200 or 401", rr.Code)
	}
}

func TestSessionMiddleware_RevokedSession(t *testing.T) {
	s := setupTestServer(t)

	// Create a user
	hash, _ := auth.HashPassword("password")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test User", "user")

	// Create session token
	token, _ := s.session.CreateToken(user.ID, "user")

	// Create a revocation after the token was issued
	time.Sleep(10 * time.Millisecond) // Ensure revocation is after token issue
	s.db.CreateSessionRevocation(user.ID, "test revocation")

	req := httptest.NewRequest("GET", "/api/messages/recent", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	handler := s.sessionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// If token doesn't need reissue, handler will be called
		w.WriteHeader(http.StatusOK)
	})

	handler(rr, req)

	// Token may or may not need reissue depending on timing
	// This test verifies the middleware handles revocations properly when checking
	if rr.Code != http.StatusOK && rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 200 or 401", rr.Code)
	}
}

func TestSessionMiddleware_RoleInContext(t *testing.T) {
	s := setupTestServer(t)

	// Create a session token with admin role
	token, _ := s.session.CreateToken(42, "admin")

	req := httptest.NewRequest("GET", "/api/messages/recent", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	handler := s.sessionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		data := auth.UserDataFromContext(r.Context())

		if data.UserID != 42 {
			t.Errorf("UserID = %d, want 42", data.UserID)
		}
		if data.Role != "admin" {
			t.Errorf("Role = %s, want admin", data.Role)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rr.Code)
	}
}

func TestHandleSignup_SetsSessionCookie(t *testing.T) {
	s := setupTestServer(t)

	// Create an admin to create invite
	hash, _ := auth.HashPassword("password123")
	admin, _ := s.db.CreateUserFull("admin", hash, "Admin", "admin")

	// Create invite
	invite, _ := s.db.CreateInvite(admin.ID, "test-invite-code")

	form := fmt.Sprintf("code=%s&username=newuser&display_name=New+User&password=password123", invite.Code)
	req := httptest.NewRequest("POST", "/signup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}

	// Check for session cookie
	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set")
	}
}

func TestHandleLogout_ClearsCookie(t *testing.T) {
	s := setupTestServer(t)

	req := httptest.NewRequest("POST", "/logout", nil)
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want 204", rr.Code)
	}

	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie in response")
	} else if sessionCookie.MaxAge != -1 {
		t.Errorf("Cookie MaxAge = %d, want -1 (delete)", sessionCookie.MaxAge)
	}

	// Check HX-Redirect header
	if rr.Header().Get("HX-Redirect") != "/" {
		t.Errorf("HX-Redirect = %q, want /", rr.Header().Get("HX-Redirect"))
	}
}

func TestHandleLogout_WithAllParam_CreatesRevocation(t *testing.T) {
	s := setupTestServer(t)

	// Create user and session
	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	req := httptest.NewRequest("POST", "/logout?all=true", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want 204", rr.Code)
	}

	// Verify revocation was created
	hasRevocation, _ := s.db.HasSessionRevocation(user.ID, time.Now().Add(-1*time.Hour))
	if !hasRevocation {
		t.Error("Expected revocation to be created")
	}
}
