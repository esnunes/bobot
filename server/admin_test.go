// server/admin_test.go
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

// adminPasswordRequest posts a password-change request to the target user as
// the given admin and returns the recorder.
func adminPasswordRequest(srv *Server, adminID, targetID int64, form string) *httptest.ResponseRecorder {
	token, _ := srv.session.CreateToken(adminID, "admin")
	req := httptest.NewRequest("POST", fmt.Sprintf("/admin/users/%d/password", targetID), strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// hasAnyRevocation reports whether any session revocation exists for the user.
func hasAnyRevocation(t *testing.T, srv *Server, userID int64) bool {
	t.Helper()
	has, err := srv.db.HasSessionRevocation(userID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("HasSessionRevocation failed: %v", err)
	}
	return has
}

func TestAdminUpdateUserPassword_Success(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	oldHash, _ := auth.HashPassword("oldpass12")
	target, _ := srv.db.CreateUserFull("target", oldHash, "Target", "user")

	w := adminPasswordRequest(srv, admin.ID, target.ID, "password=newpass12&confirm_password=newpass12")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	updated, _ := srv.db.GetUserByID(target.ID)
	if !auth.CheckPassword("newpass12", updated.PasswordHash) {
		t.Error("expected password to be updated to the new value")
	}
	if auth.CheckPassword("oldpass12", updated.PasswordHash) {
		t.Error("expected old password to no longer validate")
	}
	if !hasAnyRevocation(t, srv, target.ID) {
		t.Error("expected a session revocation to be created for the target user")
	}
}

func TestAdminUpdateUserPassword_AdminTarget(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	otherHash, _ := auth.HashPassword("oldpass12")
	other, _ := srv.db.CreateUserFull("other-admin", otherHash, "Other Admin", "admin")

	w := adminPasswordRequest(srv, admin.ID, other.ID, "password=newpass12&confirm_password=newpass12")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for admin-on-admin reset, got %d: %s", w.Code, w.Body.String())
	}
	updated, _ := srv.db.GetUserByID(other.ID)
	if !auth.CheckPassword("newpass12", updated.PasswordHash) {
		t.Error("expected target admin password to be updated")
	}
}

func TestAdminUpdateUserPassword_SelfReset(t *testing.T) {
	srv := setupTestServer(t)
	adminHash, _ := auth.HashPassword("oldpass12")
	admin, _ := srv.db.CreateUserFull("admin", adminHash, "Admin", "admin")

	w := adminPasswordRequest(srv, admin.ID, admin.ID, "password=newpass12&confirm_password=newpass12")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for self-reset, got %d: %s", w.Code, w.Body.String())
	}
	updated, _ := srv.db.GetUserByID(admin.ID)
	if !auth.CheckPassword("newpass12", updated.PasswordHash) {
		t.Error("expected admin's own password to be updated")
	}
}

func TestAdminUpdateUserPassword_NonAdminForbidden(t *testing.T) {
	srv := setupTestServer(t)
	userHash, _ := auth.HashPassword("oldpass12")
	user, _ := srv.db.CreateUserFull("user", userHash, "User", "user")
	target, _ := srv.db.CreateUserFull("target", userHash, "Target", "user")

	// Authenticate as a non-admin user.
	token, _ := srv.session.CreateToken(user.ID, "user")
	req := httptest.NewRequest("POST", fmt.Sprintf("/admin/users/%d/password", target.ID),
		strings.NewReader("password=newpass12&confirm_password=newpass12"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", w.Code)
	}
	updated, _ := srv.db.GetUserByID(target.ID)
	if !auth.CheckPassword("oldpass12", updated.PasswordHash) {
		t.Error("password must not change on a forbidden request")
	}
}

func TestAdminUpdateUserPassword_Unauthenticated(t *testing.T) {
	srv := setupTestServer(t)
	target, _ := srv.db.CreateUserFull("target", "hash", "Target", "user")

	req := httptest.NewRequest("POST", fmt.Sprintf("/admin/users/%d/password", target.ID),
		strings.NewReader("password=newpass12&confirm_password=newpass12"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a session, got %d", w.Code)
	}
}

func TestAdminUpdateUserPassword_Mismatch(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	oldHash, _ := auth.HashPassword("oldpass12")
	target, _ := srv.db.CreateUserFull("target", oldHash, "Target", "user")

	w := adminPasswordRequest(srv, admin.ID, target.ID, "password=newpass12&confirm_password=different12")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on mismatch, got %d", w.Code)
	}
	updated, _ := srv.db.GetUserByID(target.ID)
	if !auth.CheckPassword("oldpass12", updated.PasswordHash) {
		t.Error("password must not change on mismatch")
	}
	if hasAnyRevocation(t, srv, target.ID) {
		t.Error("no revocation should be created when validation fails")
	}
}

func TestAdminUpdateUserPassword_TooShort(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	oldHash, _ := auth.HashPassword("oldpass12")
	target, _ := srv.db.CreateUserFull("target", oldHash, "Target", "user")

	w := adminPasswordRequest(srv, admin.ID, target.ID, "password=short&confirm_password=short")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on too-short password, got %d", w.Code)
	}
	updated, _ := srv.db.GetUserByID(target.ID)
	if !auth.CheckPassword("oldpass12", updated.PasswordHash) {
		t.Error("password must not change when too short")
	}
	if hasAnyRevocation(t, srv, target.ID) {
		t.Error("no revocation should be created when validation fails")
	}
}

func TestAdminUpdateUserPassword_TooLong(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	oldHash, _ := auth.HashPassword("oldpass12")
	target, _ := srv.db.CreateUserFull("target", oldHash, "Target", "user")

	long := strings.Repeat("a", 73) // 73 bytes, over bcrypt's 72-byte limit
	w := adminPasswordRequest(srv, admin.ID, target.ID, "password="+long+"&confirm_password="+long)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on too-long password, got %d", w.Code)
	}
	updated, _ := srv.db.GetUserByID(target.ID)
	if !auth.CheckPassword("oldpass12", updated.PasswordHash) {
		t.Error("password must not change when too long")
	}
	if hasAnyRevocation(t, srv, target.ID) {
		t.Error("no revocation should be created when validation fails")
	}
}

func TestAdminUpdateUserPassword_UnknownID(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")

	w := adminPasswordRequest(srv, admin.ID, 99999, "password=newpass12&confirm_password=newpass12")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown user id, got %d", w.Code)
	}
}

func TestAdminUpdateUserPassword_BobotUserID(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")

	w := adminPasswordRequest(srv, admin.ID, db.BobotUserID, "password=newpass12&confirm_password=newpass12")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for the bobot user id, got %d", w.Code)
	}
}

func TestAdminUpdateUserPassword_NonNumericID(t *testing.T) {
	srv := setupTestServer(t)
	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")

	token, _ := srv.session.CreateToken(admin.ID, "admin")
	req := httptest.NewRequest("POST", "/admin/users/abc/password",
		strings.NewReader("password=newpass12&confirm_password=newpass12"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-numeric id, got %d", w.Code)
	}
}

// TestAdminUpdateUserPassword_RevokesTargetSessions exercises the full eventual
// revocation path through sessionMiddleware. It builds a server whose
// RefreshThreshold >= Duration so every request hits the reissue branch, which
// is the only place HasSessionRevocation is consulted. With the default test
// config (30m / 5m) a fresh token never triggers reissue and this path would go
// unexercised.
func TestAdminUpdateUserPassword_RevokesTargetSessions(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: 8080},
		JWT:    config.JWTConfig{Secret: "test-secret-32-chars-minimum!!"},
		Session: config.SessionConfig{
			Duration:         30 * time.Minute,
			MaxAge:           7 * 24 * time.Hour,
			RefreshThreshold: 1 * time.Hour, // >= Duration: fresh tokens always reissue
		},
	}
	srv := New(cfg, coreDB)

	admin, _ := srv.db.CreateUserFull("admin", "hash", "Admin", "admin")
	targetHash, _ := auth.HashPassword("oldpass12")
	target, _ := srv.db.CreateUserFull("target", targetHash, "Target", "user")

	adminToken, _ := srv.session.CreateToken(admin.ID, "admin")
	targetToken, _ := srv.session.CreateToken(target.ID, "user")

	// Before revocation the target token is accepted by an authenticated route.
	if code := getWithToken(srv, "/api/topics", targetToken); code == http.StatusUnauthorized {
		t.Fatalf("expected target token to be accepted before revocation, got 401")
	}

	// session_revocations.revoked_at has second resolution; sleeping past a
	// second boundary guarantees the revocation lands strictly after the
	// token's issue time, making the assertion deterministic.
	time.Sleep(1100 * time.Millisecond)

	// Admin changes the target's password.
	req := httptest.NewRequest("POST", fmt.Sprintf("/admin/users/%d/password", target.ID),
		strings.NewReader("password=newpass12&confirm_password=newpass12"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: adminToken})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// The target's pre-existing token is now rejected.
	if code := getWithToken(srv, "/api/topics", targetToken); code != http.StatusUnauthorized {
		t.Errorf("expected 401 after password change revoked sessions, got %d", code)
	}
}

func getWithToken(srv *Server, path, token string) int {
	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w.Code
}
