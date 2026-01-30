# Admin and User Roles Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement user roles (admin/user), invite system, blocking, and user management tool.

**Architecture:** Extend existing User model with role/blocked fields, add Invite table, modify JWT claims to include role, add user tool for admin operations accessible via LLM and slash commands.

**Tech Stack:** Go, SQLite, JWT (golang-jwt), bcrypt, HTML templates

---

## Task 1: Add User Fields (display_name, role, blocked)

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write failing test for User struct with new fields**

In `db/core_test.go`, add:

```go
func TestCoreDB_CreateUserWithRole(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, err := db.CreateUserFull("testuser", "hashedpass", "Test User", "admin")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.DisplayName != "Test User" {
		t.Errorf("expected display name 'Test User', got %s", user.DisplayName)
	}
	if user.Role != "admin" {
		t.Errorf("expected role 'admin', got %s", user.Role)
	}
	if user.Blocked {
		t.Error("expected user not blocked")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCoreDB_CreateUserWithRole -v`
Expected: FAIL with "undefined: db.CreateUserFull" or field errors

**Step 3: Update User struct and add migration**

In `db/core.go`, update `User` struct:

```go
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	DisplayName  string
	Role         string // "admin" or "user"
	Blocked      bool
	CreatedAt    time.Time
}
```

In `migrate()`, add after existing column migrations:

```go
// Migrate: add display_name column
if err := c.addColumnIfMissing("users", "display_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
	return err
}

// Migrate: add role column
if err := c.addColumnIfMissing("users", "role", "TEXT NOT NULL DEFAULT 'user'"); err != nil {
	return err
}

// Migrate: add blocked column
if err := c.addColumnIfMissing("users", "blocked", "INTEGER NOT NULL DEFAULT 0"); err != nil {
	return err
}
```

**Step 4: Add CreateUserFull function**

In `db/core.go`, add:

```go
func (c *CoreDB) CreateUserFull(username, passwordHash, displayName, role string) (*User, error) {
	result, err := c.db.Exec(
		"INSERT INTO users (username, password_hash, display_name, role) VALUES (?, ?, ?, ?)",
		username, passwordHash, displayName, role,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		Role:         role,
		Blocked:      false,
		CreatedAt:    time.Now(),
	}, nil
}
```

**Step 5: Update existing GetUser functions to include new fields**

Update `GetUserByUsername`:

```go
func (c *CoreDB) GetUserByUsername(username string) (*User, error) {
	var user User
	var blocked int
	err := c.db.QueryRow(
		"SELECT id, username, password_hash, display_name, role, blocked, created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.DisplayName, &user.Role, &blocked, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	user.Blocked = blocked == 1
	return &user, nil
}
```

Update `GetUserByID`:

```go
func (c *CoreDB) GetUserByID(id int64) (*User, error) {
	var user User
	var blocked int
	err := c.db.QueryRow(
		"SELECT id, username, password_hash, display_name, role, blocked, created_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.DisplayName, &user.Role, &blocked, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	user.Blocked = blocked == 1
	return &user, nil
}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./db -v`
Expected: PASS

**Step 7: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add display_name, role, blocked fields to User"
```

---

## Task 2: Add Invites Table

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write failing test for invite creation**

In `db/core_test.go`, add:

```go
func TestCoreDB_CreateInvite(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")

	invite, err := db.CreateInvite(admin.ID, "abc123")
	if err != nil {
		t.Fatalf("failed to create invite: %v", err)
	}

	if invite.Code != "abc123" {
		t.Errorf("expected code 'abc123', got %s", invite.Code)
	}
	if invite.CreatedBy != admin.ID {
		t.Errorf("expected created_by %d, got %d", admin.ID, invite.CreatedBy)
	}
	if invite.UsedBy != nil {
		t.Error("expected used_by to be nil")
	}
	if invite.Revoked {
		t.Error("expected invite not revoked")
	}
}

func TestCoreDB_GetInviteByCode(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "findme")

	invite, err := db.GetInviteByCode("findme")
	if err != nil {
		t.Fatalf("failed to get invite: %v", err)
	}
	if invite.Code != "findme" {
		t.Errorf("expected code 'findme', got %s", invite.Code)
	}
}

func TestCoreDB_UseInvite(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "useme")

	user, _ := db.CreateUserFull("newuser", "hash", "New User", "user")
	err := db.UseInvite("useme", user.ID)
	if err != nil {
		t.Fatalf("failed to use invite: %v", err)
	}

	invite, _ := db.GetInviteByCode("useme")
	if invite.UsedBy == nil || *invite.UsedBy != user.ID {
		t.Error("expected invite to be marked as used")
	}
	if invite.UsedAt == nil {
		t.Error("expected used_at to be set")
	}
}

func TestCoreDB_RevokeInvite(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "revokeme")

	err := db.RevokeInvite("revokeme")
	if err != nil {
		t.Fatalf("failed to revoke invite: %v", err)
	}

	invite, _ := db.GetInviteByCode("revokeme")
	if !invite.Revoked {
		t.Error("expected invite to be revoked")
	}
}

func TestCoreDB_GetPendingInvites(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "pending1")
	db.CreateInvite(admin.ID, "pending2")
	db.CreateInvite(admin.ID, "used")
	db.CreateInvite(admin.ID, "revoked")

	user, _ := db.CreateUserFull("user", "hash", "User", "user")
	db.UseInvite("used", user.ID)
	db.RevokeInvite("revoked")

	invites, err := db.GetPendingInvites()
	if err != nil {
		t.Fatalf("failed to get pending invites: %v", err)
	}

	if len(invites) != 2 {
		t.Errorf("expected 2 pending invites, got %d", len(invites))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCoreDB_CreateInvite -v`
Expected: FAIL with "undefined: db.CreateInvite"

**Step 3: Add Invite struct and table**

In `db/core.go`, add struct after `Message`:

```go
type Invite struct {
	ID        int64
	Code      string
	CreatedBy int64
	UsedBy    *int64
	UsedAt    *time.Time
	Revoked   bool
	CreatedAt time.Time
}
```

In `migrate()`, add after users table creation:

```go
// Create invites table
_, err = c.db.Exec(`
	CREATE TABLE IF NOT EXISTS invites (
		id INTEGER PRIMARY KEY,
		code TEXT UNIQUE NOT NULL,
		created_by INTEGER NOT NULL REFERENCES users(id),
		used_by INTEGER REFERENCES users(id),
		used_at DATETIME,
		revoked INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)
`)
if err != nil {
	return err
}
```

**Step 4: Add invite functions**

In `db/core.go`, add:

```go
func (c *CoreDB) CreateInvite(createdBy int64, code string) (*Invite, error) {
	result, err := c.db.Exec(
		"INSERT INTO invites (code, created_by) VALUES (?, ?)",
		code, createdBy,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Invite{
		ID:        id,
		Code:      code,
		CreatedBy: createdBy,
		Revoked:   false,
		CreatedAt: time.Now(),
	}, nil
}

func (c *CoreDB) GetInviteByCode(code string) (*Invite, error) {
	var invite Invite
	var usedBy sql.NullInt64
	var usedAt sql.NullTime
	var revoked int

	err := c.db.QueryRow(
		"SELECT id, code, created_by, used_by, used_at, revoked, created_at FROM invites WHERE code = ?",
		code,
	).Scan(&invite.ID, &invite.Code, &invite.CreatedBy, &usedBy, &usedAt, &revoked, &invite.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if usedBy.Valid {
		invite.UsedBy = &usedBy.Int64
	}
	if usedAt.Valid {
		invite.UsedAt = &usedAt.Time
	}
	invite.Revoked = revoked == 1

	return &invite, nil
}

func (c *CoreDB) UseInvite(code string, userID int64) error {
	result, err := c.db.Exec(
		"UPDATE invites SET used_by = ?, used_at = CURRENT_TIMESTAMP WHERE code = ? AND used_by IS NULL AND revoked = 0",
		userID, code,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (c *CoreDB) RevokeInvite(code string) error {
	result, err := c.db.Exec(
		"UPDATE invites SET revoked = 1 WHERE code = ? AND used_by IS NULL",
		code,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (c *CoreDB) GetPendingInvites() ([]Invite, error) {
	rows, err := c.db.Query(`
		SELECT i.id, i.code, i.created_by, i.revoked, i.created_at
		FROM invites i
		WHERE i.used_by IS NULL AND i.revoked = 0
		ORDER BY i.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		var inv Invite
		var revoked int
		if err := rows.Scan(&inv.ID, &inv.Code, &inv.CreatedBy, &revoked, &inv.CreatedAt); err != nil {
			return nil, err
		}
		inv.Revoked = revoked == 1
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./db -v`
Expected: PASS

**Step 6: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add invites table with create, use, revoke operations"
```

---

## Task 3: Add Block/Unblock and List Users

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write failing tests**

In `db/core_test.go`, add:

```go
func TestCoreDB_BlockUser(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUserFull("blockme", "hash", "Block Me", "user")

	err := db.BlockUser(user.ID)
	if err != nil {
		t.Fatalf("failed to block user: %v", err)
	}

	updated, _ := db.GetUserByID(user.ID)
	if !updated.Blocked {
		t.Error("expected user to be blocked")
	}
}

func TestCoreDB_UnblockUser(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUserFull("unblockme", "hash", "Unblock Me", "user")
	db.BlockUser(user.ID)

	err := db.UnblockUser(user.ID)
	if err != nil {
		t.Fatalf("failed to unblock user: %v", err)
	}

	updated, _ := db.GetUserByID(user.ID)
	if updated.Blocked {
		t.Error("expected user to be unblocked")
	}
}

func TestCoreDB_ListUsers(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	db.CreateUserFull("admin", "hash", "Admin User", "admin")
	db.CreateUserFull("user1", "hash", "User One", "user")
	db.CreateUserFull("user2", "hash", "User Two", "user")

	users, err := db.ListUsers()
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}

	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./db -run TestCoreDB_BlockUser -v`
Expected: FAIL with "undefined: db.BlockUser"

**Step 3: Add block/unblock/list functions**

In `db/core.go`, add:

```go
func (c *CoreDB) BlockUser(userID int64) error {
	_, err := c.db.Exec("UPDATE users SET blocked = 1 WHERE id = ?", userID)
	return err
}

func (c *CoreDB) UnblockUser(userID int64) error {
	_, err := c.db.Exec("UPDATE users SET blocked = 0 WHERE id = ?", userID)
	return err
}

func (c *CoreDB) ListUsers() ([]User, error) {
	rows, err := c.db.Query(`
		SELECT id, username, password_hash, display_name, role, blocked, created_at
		FROM users
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var blocked int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &blocked, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Blocked = blocked == 1
		users = append(users, u)
	}
	return users, rows.Err()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./db -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add block, unblock, and list users operations"
```

---

## Task 4: Add Role to JWT Claims

**Files:**
- Modify: `auth/jwt.go`
- Modify: `auth/jwt_test.go`

**Step 1: Write failing test for role in claims**

In `auth/jwt_test.go`, add:

```go
func TestJWTService_GenerateAccessTokenWithRole(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	token, err := svc.GenerateAccessTokenWithRole(123, "admin")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if claims.UserID != 123 {
		t.Errorf("expected user_id 123, got %d", claims.UserID)
	}
	if claims.Role != "admin" {
		t.Errorf("expected role 'admin', got %s", claims.Role)
	}
}

func TestJWTService_RoleDefaultsToEmpty(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	// Old tokens without role should still work
	token, _ := svc.GenerateAccessToken(456)
	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	// Role should be empty for backward compat
	if claims.Role != "" {
		t.Errorf("expected empty role for old token, got %s", claims.Role)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./auth -run TestJWTService_GenerateAccessTokenWithRole -v`
Expected: FAIL with "undefined: svc.GenerateAccessTokenWithRole"

**Step 3: Update Claims struct and add function**

In `auth/jwt.go`, update Claims:

```go
type Claims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role,omitempty"`
	jwt.RegisteredClaims
}
```

Add new function:

```go
func (s *JWTService) GenerateAccessTokenWithRole(userID int64, role string) (string, error) {
	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./auth -v`
Expected: PASS

**Step 5: Commit**

```bash
git add auth/jwt.go auth/jwt_test.go
git commit -m "feat(auth): add role to JWT claims"
```

---

## Task 5: Add Role to Auth Context

**Files:**
- Modify: `auth/context.go`
- Create: `auth/context_test.go`

**Step 1: Read current context.go**

Read `auth/context.go` to understand current implementation.

**Step 2: Write failing test**

Create `auth/context_test.go`:

```go
package auth

import (
	"context"
	"testing"
)

func TestContextWithRole(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithUserID(ctx, 123)
	ctx = ContextWithRole(ctx, "admin")

	if UserIDFromContext(ctx) != 123 {
		t.Error("expected user_id 123")
	}
	if RoleFromContext(ctx) != "admin" {
		t.Errorf("expected role 'admin', got %s", RoleFromContext(ctx))
	}
}

func TestRoleFromContextEmpty(t *testing.T) {
	ctx := context.Background()

	if RoleFromContext(ctx) != "" {
		t.Error("expected empty role from empty context")
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./auth -run TestContextWithRole -v`
Expected: FAIL with "undefined: ContextWithRole"

**Step 4: Add role context functions**

In `auth/context.go`, add:

```go
type roleKey struct{}

func ContextWithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleKey{}, role)
}

func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(roleKey{}).(string)
	return role
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./auth -v`
Expected: PASS

**Step 6: Commit**

```bash
git add auth/context.go auth/context_test.go
git commit -m "feat(auth): add role to request context"
```

---

## Task 6: Update Login/Refresh to Check Blocked Status

**Files:**
- Modify: `server/auth.go`
- Modify: `server/auth_test.go`

**Step 1: Write failing test for blocked user login**

In `server/auth_test.go` (create if needed), add:

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func setupTestServer(t *testing.T) (*Server, *db.CoreDB) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	jwtSvc := auth.NewJWTService("test-secret-key-32-chars-min!!")
	cfg := &config.Config{}
	srv := New(cfg, coreDB, jwtSvc)
	return srv, coreDB
}

func TestLogin_BlockedUser(t *testing.T) {
	srv, coreDB := setupTestServer(t)
	defer coreDB.Close()

	// Create and block a user
	hash, _ := auth.HashPassword("password")
	user, _ := coreDB.CreateUserFull("blocked", hash, "Blocked User", "user")
	coreDB.BlockUser(user.ID)

	// Try to login
	body, _ := json.Marshal(loginRequest{Username: "blocked", Password: "password"})
	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRefresh_BlockedUser(t *testing.T) {
	srv, coreDB := setupTestServer(t)
	defer coreDB.Close()

	// Create user and get tokens
	hash, _ := auth.HashPassword("password")
	user, _ := coreDB.CreateUserFull("toblock", hash, "To Block", "user")

	// Create refresh token
	jwtSvc := auth.NewJWTService("test-secret-key-32-chars-min!!")
	refreshToken := jwtSvc.GenerateRefreshToken()
	coreDB.CreateRefreshToken(user.ID, refreshToken, time.Now().Add(24*time.Hour))

	// Block the user
	coreDB.BlockUser(user.ID)

	// Try to refresh
	body, _ := json.Marshal(refreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest("POST", "/api/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestLogin_BlockedUser -v`
Expected: FAIL (returns 200 instead of 403)

**Step 3: Update handleLogin to check blocked status**

In `server/auth.go`, update `handleLogin` after password check:

```go
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, err := s.db.GetUserByUsername(req.Username)
	if err == db.ErrNotFound {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Check if user is blocked
	if user.Blocked {
		http.Error(w, "account blocked", http.StatusForbidden)
		return
	}

	accessToken, err := s.jwt.GenerateAccessTokenWithRole(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	refreshToken := s.jwt.GenerateRefreshToken()
	expiresAt := time.Now().Add(s.jwt.RefreshTTL())

	_, err = s.db.CreateRefreshToken(user.ID, refreshToken, expiresAt)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}
```

**Step 4: Update handleRefresh to check blocked status**

In `server/auth.go`, update `handleRefresh`:

```go
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	token, err := s.db.GetRefreshToken(req.RefreshToken)
	if err == db.ErrNotFound {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if time.Now().After(token.ExpiresAt) {
		s.db.DeleteRefreshToken(req.RefreshToken)
		http.Error(w, "token expired", http.StatusUnauthorized)
		return
	}

	// Check if user is blocked
	user, err := s.db.GetUserByID(token.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user.Blocked {
		s.db.DeleteRefreshToken(req.RefreshToken)
		http.Error(w, "account blocked", http.StatusForbidden)
		return
	}

	accessToken, err := s.jwt.GenerateAccessTokenWithRole(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
	})
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./server -v`
Expected: PASS

**Step 6: Commit**

```bash
git add server/auth.go server/auth_test.go
git commit -m "feat(auth): check blocked status on login and refresh"
```

---

## Task 7: Update Initial User to be Admin

**Files:**
- Modify: `main.go`

**Step 1: Update initial user creation to use admin role**

In `main.go`, update the initial user creation block:

```go
// Create initial user if configured and no users exist
if cfg.InitUser != "" && cfg.InitPass != "" {
	count, _ := coreDB.UserCount()
	if count == 0 {
		hash, err := auth.HashPassword(cfg.InitPass)
		if err != nil {
			log.Fatalf("Failed to hash initial password: %v", err)
		}
		_, err = coreDB.CreateUserFull(cfg.InitUser, hash, cfg.InitUser, "admin")
		if err != nil {
			log.Fatalf("Failed to create initial user: %v", err)
		}
		log.Printf("Created initial admin user: %s", cfg.InitUser)
	}
}
```

**Step 2: Run application to verify**

Run: `go build && ./bobot`
Expected: "Created initial admin user: ..." in logs

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat: make initial user an admin"
```

---

## Task 8: Create User Tool

**Files:**
- Create: `tools/user/user.go`
- Create: `tools/user/user_test.go`

**Step 1: Write failing test for user tool**

Create `tools/user/user_test.go`:

```go
package user

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	return coreDB
}

func TestUserTool_InviteCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := context.Background()
	ctx = auth.ContextWithUserID(ctx, admin.ID)
	ctx = auth.ContextWithRole(ctx, "admin")

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "invite",
	})
	if err != nil {
		t.Fatalf("failed to execute invite: %v", err)
	}

	if !strings.Contains(result, "http://localhost:8080/signup?code=") {
		t.Errorf("expected signup URL, got: %s", result)
	}
}

func TestUserTool_BlockCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	user, _ := coreDB.CreateUserFull("victim", "hash", "Victim", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := context.Background()
	ctx = auth.ContextWithUserID(ctx, admin.ID)
	ctx = auth.ContextWithRole(ctx, "admin")

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command":  "block",
		"username": "victim",
	})
	if err != nil {
		t.Fatalf("failed to execute block: %v", err)
	}

	if !strings.Contains(result, "blocked") {
		t.Errorf("expected confirmation, got: %s", result)
	}

	// Verify user is blocked
	updated, _ := coreDB.GetUserByID(user.ID)
	if !updated.Blocked {
		t.Error("expected user to be blocked")
	}
}

func TestUserTool_NonAdminDenied(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("user", "hash", "User", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := context.Background()
	ctx = auth.ContextWithUserID(ctx, user.ID)
	ctx = auth.ContextWithRole(ctx, "user")

	_, err := tool.Execute(ctx, map[string]interface{}{
		"command": "list",
	})
	if err == nil {
		t.Error("expected error for non-admin")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin error, got: %v", err)
	}
}

func TestUserTool_CannotBlockSelf(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := context.Background()
	ctx = auth.ContextWithUserID(ctx, admin.ID)
	ctx = auth.ContextWithRole(ctx, "admin")

	_, err := tool.Execute(ctx, map[string]interface{}{
		"command":  "block",
		"username": "admin",
	})
	if err == nil {
		t.Error("expected error when blocking self")
	}
}

func TestUserTool_ListCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateUserFull("user1", "hash", "User One", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := context.Background()
	ctx = auth.ContextWithUserID(ctx, admin.ID)
	ctx = auth.ContextWithRole(ctx, "admin")

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "list",
	})
	if err != nil {
		t.Fatalf("failed to execute list: %v", err)
	}

	if !strings.Contains(result, "admin") || !strings.Contains(result, "user1") {
		t.Errorf("expected user list, got: %s", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./tools/user -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create user tool**

Create `tools/user/user.go`:

```go
package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type UserTool struct {
	db      *db.CoreDB
	baseURL string
}

func NewUserTool(db *db.CoreDB, baseURL string) *UserTool {
	return &UserTool{db: db, baseURL: baseURL}
}

func (t *UserTool) Name() string {
	return "user"
}

func (t *UserTool) Description() string {
	return "Manage users: invite new users, block/unblock users, list users and invites"
}

func (t *UserTool) Schema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"invite", "block", "unblock", "list", "invites", "revoke"},
				"description": "The operation to perform",
			},
			"username": map[string]interface{}{
				"type":        "string",
				"description": "Username for block/unblock commands",
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "Invite code for revoke command",
			},
		},
		"required": []string{"command"},
	}
}

func (t *UserTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	// Check admin role
	role := auth.RoleFromContext(ctx)
	if role != "admin" {
		return "", fmt.Errorf("this command requires admin privileges")
	}

	userID := auth.UserIDFromContext(ctx)
	command, _ := input["command"].(string)
	username, _ := input["username"].(string)
	code, _ := input["code"].(string)

	switch command {
	case "invite":
		return t.invite(userID)
	case "block":
		return t.block(userID, username)
	case "unblock":
		return t.unblock(username)
	case "list":
		return t.list()
	case "invites":
		return t.listInvites()
	case "revoke":
		return t.revoke(code)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *UserTool) invite(createdBy int64) (string, error) {
	code := generateInviteCode()
	_, err := t.db.CreateInvite(createdBy, code)
	if err != nil {
		return "", fmt.Errorf("failed to create invite: %w", err)
	}

	url := fmt.Sprintf("%s/signup?code=%s", t.baseURL, code)
	return fmt.Sprintf("Invite created! Share this signup URL:\n%s", url), nil
}

func (t *UserTool) block(adminID int64, username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	user, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if user.ID == adminID {
		return "", fmt.Errorf("cannot block yourself")
	}

	if err := t.db.BlockUser(user.ID); err != nil {
		return "", err
	}

	// Delete all refresh tokens for blocked user
	if err := t.db.DeleteUserRefreshTokens(user.ID); err != nil {
		return "", err
	}

	return fmt.Sprintf("User '%s' has been blocked.", username), nil
}

func (t *UserTool) unblock(username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	user, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if err := t.db.UnblockUser(user.ID); err != nil {
		return "", err
	}

	return fmt.Sprintf("User '%s' has been unblocked.", username), nil
}

func (t *UserTool) list() (string, error) {
	users, err := t.db.ListUsers()
	if err != nil {
		return "", err
	}

	if len(users) == 0 {
		return "No users found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Users:\n")
	sb.WriteString("| Username | Display Name | Role | Status | Created |\n")
	sb.WriteString("|----------|--------------|------|--------|--------|\n")
	for _, u := range users {
		status := "active"
		if u.Blocked {
			status = "blocked"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			u.Username, u.DisplayName, u.Role, status, u.CreatedAt.Format("2006-01-02")))
	}
	return sb.String(), nil
}

func (t *UserTool) listInvites() (string, error) {
	invites, err := t.db.GetPendingInvites()
	if err != nil {
		return "", err
	}

	if len(invites) == 0 {
		return "No pending invites.", nil
	}

	var sb strings.Builder
	sb.WriteString("Pending Invites:\n")
	sb.WriteString("| Code | Created By | Created At |\n")
	sb.WriteString("|------|------------|------------|\n")
	for _, inv := range invites {
		creator, _ := t.db.GetUserByID(inv.CreatedBy)
		creatorName := "unknown"
		if creator != nil {
			creatorName = creator.Username
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
			inv.Code, creatorName, inv.CreatedAt.Format("2006-01-02 15:04")))
	}
	return sb.String(), nil
}

func (t *UserTool) revoke(code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("invite code is required")
	}

	err := t.db.RevokeInvite(code)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("invite not found or already used")
	}
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Invite '%s' has been revoked.", code), nil
}

func generateInviteCode() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./tools/user -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tools/user/user.go tools/user/user_test.go
git commit -m "feat(tools): add user tool for admin operations"
```

---

## Task 9: Add Signup Endpoint and Page

**Files:**
- Modify: `server/auth.go`
- Modify: `server/server.go`
- Create: `web/templates/signup.html`

**Step 1: Write failing test for signup**

In `server/auth_test.go`, add:

```go
func TestSignup_ValidInvite(t *testing.T) {
	srv, coreDB := setupTestServer(t)
	defer coreDB.Close()

	// Create admin and invite
	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateInvite(admin.ID, "validcode")

	body, _ := json.Marshal(signupRequest{
		Code:        "validcode",
		Username:    "newuser",
		DisplayName: "New User",
		Password:    "password123",
	})
	req := httptest.NewRequest("POST", "/api/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify user was created
	user, err := coreDB.GetUserByUsername("newuser")
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
	invite, _ := coreDB.GetInviteByCode("validcode")
	if invite.UsedBy == nil {
		t.Error("invite should be marked as used")
	}
}

func TestSignup_InvalidInvite(t *testing.T) {
	srv, coreDB := setupTestServer(t)
	defer coreDB.Close()

	body, _ := json.Marshal(signupRequest{
		Code:        "invalidcode",
		Username:    "newuser",
		DisplayName: "New User",
		Password:    "password123",
	})
	req := httptest.NewRequest("POST", "/api/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSignup_UsernameValidation(t *testing.T) {
	srv, coreDB := setupTestServer(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateInvite(admin.ID, "testcode")

	// Too short
	body, _ := json.Marshal(signupRequest{
		Code:        "testcode",
		Username:    "ab",
		DisplayName: "Test",
		Password:    "password123",
	})
	req := httptest.NewRequest("POST", "/api/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short username, got %d", w.Code)
	}
}

func TestSignup_PasswordValidation(t *testing.T) {
	srv, coreDB := setupTestServer(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateInvite(admin.ID, "testcode2")

	// Too short
	body, _ := json.Marshal(signupRequest{
		Code:        "testcode2",
		Username:    "validuser",
		DisplayName: "Test",
		Password:    "short",
	})
	req := httptest.NewRequest("POST", "/api/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestSignup -v`
Expected: FAIL (endpoint doesn't exist)

**Step 3: Add signup request struct and handler**

In `server/auth.go`, add struct:

```go
type signupRequest struct {
	Code        string `json:"code"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}
```

Add validation helpers:

```go
import (
	"regexp"
	"strings"
)

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func validateUsername(username string) error {
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters")
	}
	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("username can only contain letters, numbers, and underscores")
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	return nil
}

func validateDisplayName(name string) error {
	if len(strings.TrimSpace(name)) < 1 {
		return fmt.Errorf("display name is required")
	}
	return nil
}
```

Add handler:

```go
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Validate invite code
	invite, err := s.db.GetInviteByCode(req.Code)
	if err == db.ErrNotFound {
		http.Error(w, "invalid invite code", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if invite.UsedBy != nil || invite.Revoked {
		http.Error(w, "invite code already used or revoked", http.StatusBadRequest)
		return
	}

	// Validate inputs
	req.Username = strings.ToLower(req.Username)
	if err := validateUsername(req.Username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validatePassword(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateDisplayName(req.DisplayName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check username uniqueness
	_, err = s.db.GetUserByUsername(req.Username)
	if err == nil {
		http.Error(w, "username already taken", http.StatusBadRequest)
		return
	}
	if err != db.ErrNotFound {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Create user
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	user, err := s.db.CreateUserFull(req.Username, hash, req.DisplayName, "user")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Mark invite as used
	if err := s.db.UseInvite(req.Code, user.ID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Generate tokens
	accessToken, err := s.jwt.GenerateAccessTokenWithRole(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	refreshToken := s.jwt.GenerateRefreshToken()
	expiresAt := time.Now().Add(s.jwt.RefreshTTL())

	_, err = s.db.CreateRefreshToken(user.ID, refreshToken, expiresAt)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}
```

**Step 4: Add route in server.go**

In `server/server.go`, add route in the appropriate place:

```go
mux.HandleFunc("POST /api/signup", s.handleSignup)
mux.HandleFunc("GET /signup", s.handleSignupPage)
```

Add signup page handler:

```go
func (s *Server) handleSignupPage(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "invite code required", http.StatusBadRequest)
		return
	}

	// Validate code exists and is valid
	invite, err := s.db.GetInviteByCode(code)
	if err == db.ErrNotFound || invite.UsedBy != nil || invite.Revoked {
		s.renderTemplate(w, "signup", map[string]interface{}{
			"Error": "Invalid or expired invite",
			"Code":  "",
		})
		return
	}

	s.renderTemplate(w, "signup", map[string]interface{}{
		"Code": code,
	})
}
```

**Step 5: Create signup template**

Create `web/templates/signup.html`:

```html
{{define "content"}}
<div class="login-container">
    <h1>bobot</h1>
    {{if .Error}}
    <p class="error">{{.Error}}</p>
    {{else}}
    <form id="signup-form" class="login-form">
        <input type="hidden" name="code" value="{{.Code}}">
        <input type="text" name="username" placeholder="Username" required autofocus
               pattern="[a-zA-Z0-9_]+" minlength="3" title="Letters, numbers, and underscores only">
        <input type="text" name="display_name" placeholder="Display Name" required>
        <input type="password" name="password" placeholder="Password" required minlength="8">
        <input type="password" name="confirm_password" placeholder="Confirm Password" required>
        <button type="submit">Sign Up</button>
        <p id="error-message" class="error hidden"></p>
    </form>
    {{end}}
</div>
<script>
document.getElementById('signup-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const form = e.target;
    const errorEl = document.getElementById('error-message');

    if (form.password.value !== form.confirm_password.value) {
        errorEl.textContent = 'Passwords do not match';
        errorEl.classList.remove('hidden');
        return;
    }

    try {
        const resp = await fetch('/api/signup', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                code: form.code.value,
                username: form.username.value,
                display_name: form.display_name.value,
                password: form.password.value
            })
        });

        if (!resp.ok) {
            const text = await resp.text();
            throw new Error(text || 'Signup failed');
        }

        const data = await resp.json();
        localStorage.setItem('access_token', data.access_token);
        localStorage.setItem('refresh_token', data.refresh_token);
        window.location.href = '/chat';
    } catch (err) {
        errorEl.textContent = err.message;
        errorEl.classList.remove('hidden');
    }
});
</script>
{{end}}
```

**Step 6: Run tests to verify they pass**

Run: `go test ./server -v`
Expected: PASS

**Step 7: Commit**

```bash
git add server/auth.go server/server.go web/templates/signup.html
git commit -m "feat: add signup endpoint and page with invite validation"
```

---

## Task 10: Register User Tool and Add Role to Middleware Context

**Files:**
- Modify: `main.go`
- Modify: `server/middleware.go`
- Modify: `config/config.go`

**Step 1: Add base URL to config**

In `config/config.go`, add:

```go
type Config struct {
	// ... existing fields
	BaseURL string
}

// In Load():
cfg.BaseURL = getEnv("BOBOT_BASE_URL", "http://localhost:8080")
```

**Step 2: Update middleware to set role in context**

In `server/middleware.go`, update the auth middleware to include role:

```go
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ... existing token extraction ...

		claims, err := s.jwt.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := auth.ContextWithUserID(r.Context(), claims.UserID)
		ctx = auth.ContextWithRole(ctx, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

**Step 3: Register user tool in main.go**

In `main.go`, add import and registration:

```go
import (
	// ... existing imports
	"github.com/esnunes/bobot/tools/user"
)

// After registry creation:
registry.Register(task.NewTaskTool(taskDB))
registry.Register(user.NewUserTool(coreDB, cfg.BaseURL))
```

**Step 4: Run all tests**

Run: `go test ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add main.go server/middleware.go config/config.go
git commit -m "feat: register user tool and add role to auth context"
```

---

## Task 11: Conditional Tool Loading for Admin

**Files:**
- Modify: `assistant/engine.go`
- Modify: `tools/registry.go`

**Step 1: Write failing test**

In `assistant/engine_test.go`, add test for conditional tools (or create the file if it doesn't exist).

**Step 2: Add AdminOnly method to Tool interface**

In `tools/registry.go`, update interface:

```go
type Tool interface {
	Name() string
	Description() string
	Schema() interface{}
	Execute(ctx context.Context, input map[string]interface{}) (string, error)
	AdminOnly() bool // NEW
}
```

Update `ToLLMTools` to accept role:

```go
func (r *Registry) ToLLMToolsForRole(role string) []llm.Tool {
	result := make([]llm.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		if tool.AdminOnly() && role != "admin" {
			continue
		}
		result = append(result, llm.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		})
	}
	return result
}
```

**Step 3: Update existing tools to implement AdminOnly**

In `tools/task/task.go`, add:

```go
func (t *TaskTool) AdminOnly() bool {
	return false
}
```

In `tools/user/user.go`, add:

```go
func (t *UserTool) AdminOnly() bool {
	return true
}
```

**Step 4: Update engine to use role-based tool filtering**

In `assistant/engine.go`, update Chat method:

```go
func (e *Engine) Chat(ctx context.Context, message string) (string, error) {
	// Get role from context
	role := auth.RoleFromContext(ctx)

	// Build system prompt with role-filtered tools
	llmTools := e.registry.ToLLMToolsForRole(role)
	systemPrompt := BuildSystemPrompt(e.skills, llmTools)

	// ... rest of method unchanged
}
```

**Step 5: Run tests**

Run: `go test ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add assistant/engine.go tools/registry.go tools/task/task.go tools/user/user.go
git commit -m "feat: conditional tool loading based on user role"
```

---

## Task 12: Final Integration Test

**Files:**
- Run manual test

**Step 1: Build and run**

```bash
go build -o bobot
BOBOT_INIT_USER=admin BOBOT_INIT_PASS=admin123 BOBOT_JWT_SECRET=secret123456789012345678901234 ./bobot
```

**Step 2: Test flow**

1. Login as admin
2. Use `/user invite` to create invite
3. Open signup URL in incognito
4. Create new user
5. Verify new user can chat
6. As admin, use `/user block <username>`
7. Verify blocked user's session expires
8. Verify blocked user can't login
9. Use `/user unblock <username>`
10. Verify user can login again

**Step 3: Commit any fixes**

```bash
git add .
git commit -m "fix: integration test fixes"
```

---

## Summary

This plan implements:
1. User model with display_name, role, blocked fields
2. Invites table with create, use, revoke operations
3. JWT claims with role
4. Auth context with role
5. Blocked user checks on login/refresh
6. Initial user as admin
7. User tool with invite, block, unblock, list, invites, revoke commands
8. Signup endpoint and page
9. Conditional tool loading (user tool only for admins)

All changes follow TDD with tests before implementation, and frequent commits after each task.
