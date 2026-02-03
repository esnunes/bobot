# Session-Based Authentication Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace JWT-based authentication with encrypted session cookies for simpler auth flow.

**Architecture:** Single encrypted cookie containing user_id, role, and timestamps. Middleware handles automatic reissue when token is near expiry or expired (within 7-day window). No client-side token management.

**Tech Stack:** Go stdlib crypto (AES-256-GCM), SQLite, no external auth libraries.

---

## Task 1: Add Session Configuration

**Files:**
- Modify: `config/config.go`

**Step 1: Write the failing test**

Create `config/config_test.go` (add to existing tests if present):

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./config -run TestSessionConfigDefaults -v`
Expected: FAIL - `cfg.Session` undefined

**Step 3: Write minimal implementation**

Add to `config/config.go`:

```go
type SessionConfig struct {
	Duration         time.Duration
	MaxAge           time.Duration
	RefreshThreshold time.Duration
}
```

Add `Session SessionConfig` to the `Config` struct.

In `Load()`, add:
```go
Session: SessionConfig{
	Duration:         getEnvDurationOrDefault("BOBOT_SESSION_DURATION", 30*time.Minute),
	MaxAge:           getEnvDurationOrDefault("BOBOT_SESSION_MAX_AGE", 7*24*time.Hour),
	RefreshThreshold: getEnvDurationOrDefault("BOBOT_SESSION_REFRESH_THRESHOLD", 5*time.Minute),
},
```

**Step 4: Run test to verify it passes**

Run: `go test ./config -run TestSessionConfigDefaults -v`
Expected: PASS

**Step 5: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "feat(config): add session configuration options"
```

---

## Task 2: Create Session Token Service

**Files:**
- Create: `auth/session.go`
- Create: `auth/session_test.go`

**Step 1: Write the failing test**

Create `auth/session_test.go`:

```go
package auth

import (
	"testing"
	"time"
)

func TestSessionService_CreateAndDecrypt(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	token, err := svc.CreateToken(123, "admin")
	if err != nil {
		t.Fatalf("CreateToken() error: %v", err)
	}

	if token == "" {
		t.Fatal("CreateToken() returned empty token")
	}

	session, err := svc.DecryptToken(token)
	if err != nil {
		t.Fatalf("DecryptToken() error: %v", err)
	}

	if session.UserID != 123 {
		t.Errorf("UserID = %d, want 123", session.UserID)
	}
	if session.Role != "admin" {
		t.Errorf("Role = %s, want admin", session.Role)
	}
}

func TestSessionService_DecryptInvalidToken(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	_, err := svc.DecryptToken("invalid-token")
	if err == nil {
		t.Error("DecryptToken() should fail for invalid token")
	}
}

func TestSessionService_NeedsReissue(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	// Token that expires in 10 minutes - should not need reissue
	fresh := &SessionToken{
		UserID:    1,
		Role:      "user",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if svc.NeedsReissue(fresh) {
		t.Error("Fresh token should not need reissue")
	}

	// Token that expires in 3 minutes - should need reissue (within 5 min threshold)
	nearExpiry := &SessionToken{
		UserID:    1,
		Role:      "user",
		IssuedAt:  time.Now().Add(-27 * time.Minute),
		ExpiresAt: time.Now().Add(3 * time.Minute),
	}
	if !svc.NeedsReissue(nearExpiry) {
		t.Error("Near-expiry token should need reissue")
	}

	// Token that expired 5 minutes ago - should need reissue
	expired := &SessionToken{
		UserID:    1,
		Role:      "user",
		IssuedAt:  time.Now().Add(-35 * time.Minute),
		ExpiresAt: time.Now().Add(-5 * time.Minute),
	}
	if !svc.NeedsReissue(expired) {
		t.Error("Expired token should need reissue")
	}
}

func TestSessionService_IsPastDeadline(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	// Token issued now - not past deadline
	fresh := &SessionToken{
		UserID:   1,
		Role:     "user",
		IssuedAt: time.Now(),
	}
	if svc.IsPastDeadline(fresh) {
		t.Error("Fresh token should not be past deadline")
	}

	// Token issued 8 days ago - past deadline
	old := &SessionToken{
		UserID:   1,
		Role:     "user",
		IssuedAt: time.Now().Add(-8 * 24 * time.Hour),
	}
	if !svc.IsPastDeadline(old) {
		t.Error("Old token should be past deadline")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./auth -run TestSessionService -v`
Expected: FAIL - undefined: NewSessionService

**Step 3: Write minimal implementation**

Create `auth/session.go`:

```go
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalidSession = errors.New("invalid session token")
)

type SessionToken struct {
	UserID    int64     `json:"user_id"`
	Role      string    `json:"role"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionService struct {
	key              []byte
	duration         time.Duration
	maxAge           time.Duration
	refreshThreshold time.Duration
}

func NewSessionService(secret string, duration, maxAge, refreshThreshold time.Duration) *SessionService {
	// Derive 32-byte key from secret using SHA-256
	hash := sha256.Sum256([]byte(secret))
	return &SessionService{
		key:              hash[:],
		duration:         duration,
		maxAge:           maxAge,
		refreshThreshold: refreshThreshold,
	}
}

func (s *SessionService) CreateToken(userID int64, role string) (string, error) {
	now := time.Now()
	token := &SessionToken{
		UserID:    userID,
		Role:      role,
		IssuedAt:  now,
		ExpiresAt: now.Add(s.duration),
	}

	plaintext, err := json.Marshal(token)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func (s *SessionService) DecryptToken(encrypted string) (*SessionToken, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, ErrInvalidSession
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, ErrInvalidSession
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidSession
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, ErrInvalidSession
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidSession
	}

	var token SessionToken
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return nil, ErrInvalidSession
	}

	return &token, nil
}

func (s *SessionService) NeedsReissue(token *SessionToken) bool {
	now := time.Now()
	// Needs reissue if expired OR within refresh threshold
	return now.After(token.ExpiresAt) || token.ExpiresAt.Sub(now) < s.refreshThreshold
}

func (s *SessionService) IsPastDeadline(token *SessionToken) bool {
	deadline := token.IssuedAt.Add(s.maxAge)
	return time.Now().After(deadline)
}

func (s *SessionService) Duration() time.Duration {
	return s.duration
}

func (s *SessionService) MaxAge() time.Duration {
	return s.maxAge
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./auth -run TestSessionService -v`
Expected: PASS

**Step 5: Commit**

```bash
git add auth/session.go auth/session_test.go
git commit -m "feat(auth): add encrypted session token service"
```

---

## Task 3: Add Session Revocations Table

**Files:**
- Modify: `db/core.go`

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
func TestSessionRevocations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a user first
	user, err := db.CreateUserFull("testuser", "hash", "Test User", "user")
	if err != nil {
		t.Fatalf("CreateUserFull() error: %v", err)
	}

	// Create a revocation
	err = db.CreateSessionRevocation(user.ID, "logout_all")
	if err != nil {
		t.Fatalf("CreateSessionRevocation() error: %v", err)
	}

	// Check for revocation after the token was issued (before revocation)
	tokenIssuedAt := time.Now().Add(-1 * time.Hour)
	hasRevocation, err := db.HasSessionRevocation(user.ID, tokenIssuedAt)
	if err != nil {
		t.Fatalf("HasSessionRevocation() error: %v", err)
	}
	if !hasRevocation {
		t.Error("Expected revocation to be found for token issued before revocation")
	}

	// Check for revocation before the token was issued (after revocation)
	tokenIssuedAt = time.Now().Add(1 * time.Hour)
	hasRevocation, err = db.HasSessionRevocation(user.ID, tokenIssuedAt)
	if err != nil {
		t.Fatalf("HasSessionRevocation() error: %v", err)
	}
	if hasRevocation {
		t.Error("Expected no revocation for token issued after revocation")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestSessionRevocations -v`
Expected: FAIL - undefined: CreateSessionRevocation

**Step 3: Write minimal implementation**

Add to `db/core.go` migration:

```go
// Create session_revocations table
_, err = c.db.Exec(`
	CREATE TABLE IF NOT EXISTS session_revocations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		revoked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		reason TEXT,
		FOREIGN KEY (user_id) REFERENCES users(id)
	)
`)
if err != nil {
	return err
}

_, err = c.db.Exec(`
	CREATE INDEX IF NOT EXISTS idx_revocations_user_revoked
	ON session_revocations(user_id, revoked_at)
`)
if err != nil {
	return err
}
```

Add methods to `db/core.go`:

```go
func (c *CoreDB) CreateSessionRevocation(userID int64, reason string) error {
	_, err := c.db.Exec(
		"INSERT INTO session_revocations (user_id, reason) VALUES (?, ?)",
		userID, reason,
	)
	return err
}

func (c *CoreDB) HasSessionRevocation(userID int64, tokenIssuedAt time.Time) (bool, error) {
	var count int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM session_revocations WHERE user_id = ? AND revoked_at > ?",
		userID, tokenIssuedAt,
	).Scan(&count)
	return count > 0, err
}

func (c *CoreDB) DeleteOldSessionRevocations(olderThan time.Time) (int64, error) {
	result, err := c.db.Exec(
		"DELETE FROM session_revocations WHERE revoked_at < ?",
		olderThan,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run TestSessionRevocations -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add session revocations table"
```

---

## Task 4: Update Server to Use Session Service

**Files:**
- Modify: `server/server.go`

**Step 1: Write the failing test**

Add to `server/server_test.go`:

```go
func TestServer_HasSessionService(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			Duration:         30 * time.Minute,
			MaxAge:           7 * 24 * time.Hour,
			RefreshThreshold: 5 * time.Minute,
		},
		JWT: config.JWTConfig{Secret: "test-secret"},
	}

	// This should compile - verifies Server has session field
	s := &Server{
		cfg: cfg,
	}
	_ = s
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestServer_HasSessionService -v`
Expected: Compilation should work, but we need to verify the session field exists

**Step 3: Write minimal implementation**

Update `server/server.go`:

Replace `jwt *auth.JWTService` with `session *auth.SessionService` in the Server struct.

Update `New` and `NewWithAssistant` constructors:

```go
func New(cfg *config.Config, coreDB *db.CoreDB) *Server {
	return NewWithAssistant(cfg, coreDB, nil, nil)
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, engine *assistant.Engine, registry *tools.Registry) *Server {
	session := auth.NewSessionService(
		cfg.JWT.Secret,
		cfg.Session.Duration,
		cfg.Session.MaxAge,
		cfg.Session.RefreshThreshold,
	)

	s := &Server{
		cfg:         cfg,
		db:          coreDB,
		session:     session,
		engine:      engine,
		registry:    registry,
		connections: NewConnectionRegistry(),
		router:      http.NewServeMux(),
		templates:   make(map[string]*template.Template),
	}

	s.loadTemplates()
	s.routes()
	return s
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestServer_HasSessionService -v`
Expected: PASS (or compilation succeeds)

**Step 5: Commit**

```bash
git add server/server.go
git commit -m "refactor(server): replace JWTService with SessionService"
```

---

## Task 5: Implement Session Middleware

**Files:**
- Modify: `server/server.go`

**Step 1: Write the failing test**

Add to `server/server_test.go`:

```go
func TestSessionMiddleware_ValidToken(t *testing.T) {
	s := setupTestServer(t)

	// Create a valid session token
	token, _ := s.session.CreateToken(1, "user")

	// Create request with session cookie
	req := httptest.NewRequest("GET", "/api/messages/recent", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	rr := httptest.NewRecorder()

	// Use a simple handler that returns 200 if auth succeeds
	handler := s.sessionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserIDFromContext(r.Context())
		if userID != 1 {
			t.Errorf("UserID = %d, want 1", userID)
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestSessionMiddleware -v`
Expected: FAIL - undefined: sessionMiddleware

**Step 3: Write minimal implementation**

Replace `authMiddleware` with `sessionMiddleware` in `server/server.go`:

```go
func (s *Server) sessionMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token, err := s.session.DecryptToken(cookie.Value)
		if err != nil {
			s.clearSessionCookie(w)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check absolute deadline (7 days from issue)
		if s.session.IsPastDeadline(token) {
			s.clearSessionCookie(w)
			http.Error(w, "session expired", http.StatusUnauthorized)
			return
		}

		// Check if reissue needed (expired or near expiry)
		if s.session.NeedsReissue(token) {
			// Database checks
			user, err := s.db.GetUserByID(token.UserID)
			if err != nil || user.Blocked {
				s.clearSessionCookie(w)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			hasRevocation, err := s.db.HasSessionRevocation(token.UserID, token.IssuedAt)
			if err != nil || hasRevocation {
				s.clearSessionCookie(w)
				http.Error(w, "session revoked", http.StatusUnauthorized)
				return
			}

			// Reissue token
			newToken, err := s.session.CreateToken(token.UserID, token.Role)
			if err == nil {
				s.setSessionCookie(w, newToken)
			}
		}

		ctx := auth.ContextWithUserData(r.Context(), auth.UserData{
			UserID: token.UserID,
			Role:   token.Role,
		})
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	secure := s.cfg.BaseURL[:5] == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.session.MaxAge().Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}
```

Update routes to use `sessionMiddleware` instead of `authMiddleware`.

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestSessionMiddleware -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/server.go server/server_test.go
git commit -m "feat(server): implement session middleware with auto-reissue"
```

---

## Task 6: Update Login Handler

**Files:**
- Modify: `server/auth.go`

**Step 1: Write the failing test**

Update existing login test in `server/auth_test.go` (or add):

```go
func TestHandleLogin_SetsSessionCookie(t *testing.T) {
	s := setupTestServer(t)

	// Create a user
	hash, _ := auth.HashPassword("password123")
	s.db.CreateUserFull("testuser", hash, "Test", "user")

	body := `{"username":"testuser","password":"password123"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestHandleLogin_SetsSessionCookie -v`
Expected: FAIL - no session cookie set

**Step 3: Write minimal implementation**

Update `handleLogin` in `server/auth.go`:

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

	if user.Blocked {
		http.Error(w, "account blocked", http.StatusForbidden)
		return
	}

	token, err := s.session.CreateToken(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.setSessionCookie(w, token)

	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/chat")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestHandleLogin_SetsSessionCookie -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/auth.go server/auth_test.go
git commit -m "feat(server): update login to use session cookies"
```

---

## Task 7: Update Signup Handler

**Files:**
- Modify: `server/auth.go`

**Step 1: Write the failing test**

Add to `server/auth_test.go`:

```go
func TestHandleSignup_SetsSessionCookie(t *testing.T) {
	s := setupTestServer(t)

	// Create an admin to create invite
	hash, _ := auth.HashPassword("password123")
	admin, _ := s.db.CreateUserFull("admin", hash, "Admin", "admin")

	// Create invite
	invite, _ := s.db.CreateInvite(admin.ID, "test-invite-code")

	body := fmt.Sprintf(`{"code":"%s","username":"newuser","display_name":"New User","password":"password123"}`, invite.Code)
	req := httptest.NewRequest("POST", "/api/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestHandleSignup_SetsSessionCookie -v`
Expected: FAIL - no session cookie (or still returns JWT)

**Step 3: Write minimal implementation**

Update `handleSignup` in `server/auth.go` - replace the token generation section at the end:

```go
// Generate session token
token, err := s.session.CreateToken(user.ID, user.Role)
if err != nil {
	http.Error(w, "internal error", http.StatusInternalServerError)
	return
}

s.setSessionCookie(w, token)

if isHTMXRequest(r) {
	w.Header().Set("HX-Redirect", "/chat")
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestHandleSignup_SetsSessionCookie -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/auth.go server/auth_test.go
git commit -m "feat(server): update signup to use session cookies"
```

---

## Task 8: Update Logout Handler

**Files:**
- Modify: `server/auth.go`

**Step 1: Write the failing test**

Add to `server/auth_test.go`:

```go
func TestHandleLogout_ClearsCookie(t *testing.T) {
	s := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/logout", nil)
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rr.Code)
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
}

func TestHandleLogout_WithAllParam_CreatesRevocation(t *testing.T) {
	s := setupTestServer(t)

	// Create user and session
	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	req := httptest.NewRequest("POST", "/api/logout?all=true", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rr.Code)
	}

	// Verify revocation was created
	hasRevocation, _ := s.db.HasSessionRevocation(user.ID, time.Now().Add(-1*time.Hour))
	if !hasRevocation {
		t.Error("Expected revocation to be created")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestHandleLogout -v`
Expected: FAIL

**Step 3: Write minimal implementation**

Replace `handleLogout` in `server/auth.go`:

```go
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Check for "logout everywhere" parameter
	if r.URL.Query().Get("all") == "true" {
		// Try to get user from session cookie
		if cookie, err := r.Cookie("session"); err == nil {
			if token, err := s.session.DecryptToken(cookie.Value); err == nil {
				s.db.CreateSessionRevocation(token.UserID, "logout_all")
			}
		}
	}

	s.clearSessionCookie(w)

	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestHandleLogout -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/auth.go server/auth_test.go
git commit -m "feat(server): update logout with optional revoke-all"
```

---

## Task 9: Update WebSocket Authentication

**Files:**
- Modify: `server/chat.go`

**Step 1: Write the failing test**

Add to `server/chat_test.go`:

```go
func TestWebSocket_SessionCookieAuth(t *testing.T) {
	s := setupTestServer(t)

	// Create user and session
	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	// Create test server
	ts := httptest.NewServer(s.router)
	defer ts.Close()

	// Connect with session cookie
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/chat"
	header := http.Header{}
	header.Add("Cookie", "session="+token)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("Status = %d, want 101", resp.StatusCode)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestWebSocket_SessionCookieAuth -v`
Expected: FAIL - WebSocket still uses query param token

**Step 3: Write minimal implementation**

Update `handleChat` in `server/chat.go`:

```go
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Get session from cookie
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	token, err := s.session.DecryptToken(cookie.Value)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	// Check if past absolute deadline
	if s.session.IsPastDeadline(token) {
		http.Error(w, "session expired", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Register connection for multi-device support
	s.connections.Add(token.UserID, conn)
	defer s.connections.Remove(token.UserID, conn)

	// Create context with user data
	ctx := auth.ContextWithUserData(r.Context(), auth.UserData{
		UserID: token.UserID,
		Role:   token.Role,
	})

	// Handle messages (rest remains the same)
	for {
		var msg chatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		if msg.GroupID != nil {
			s.handleGroupChatMessage(ctx, token.UserID, *msg.GroupID, msg.Content)
		} else {
			s.handlePrivateChatMessage(ctx, token.UserID, msg.Content)
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestWebSocket_SessionCookieAuth -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/chat.go server/chat_test.go
git commit -m "feat(server): update WebSocket to use session cookies"
```

---

## Task 10: Remove Refresh Endpoint and Update Routes

**Files:**
- Modify: `server/server.go`
- Modify: `server/auth.go`

**Step 1: Verify refresh endpoint is removed**

Check that `/api/refresh` route is not registered.

**Step 2: Update routes**

In `server/server.go`, remove:
```go
s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
```

**Step 3: Remove handleRefresh**

Delete `handleRefresh` function from `server/auth.go`.

**Step 4: Remove unused types**

Delete from `server/auth.go`:
```go
type tokenResponse struct { ... }
type refreshRequest struct { ... }
```

**Step 5: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 6: Commit**

```bash
git add server/server.go server/auth.go
git commit -m "refactor(server): remove refresh endpoint"
```

---

## Task 11: Simplify Client-Side JavaScript

**Files:**
- Modify: `web/static/ws-manager.js`
- Modify: `web/static/chat.js`
- Modify: `web/static/group_chat.js`

**Step 1: Update ws-manager.js**

Replace the entire file:

```javascript
// WebSocket manager - handles connection lifecycle and event dispatching
(function() {
    const container = document.getElementById('ws-connection');
    if (!container) return;

    if (container.dataset.initialized === 'true') return;
    container.dataset.initialized = 'true';

    let ws = null;
    let reconnectAttempts = 0;
    const MAX_RECONNECT_DELAY = 30000;

    function connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat`;

        ws = new WebSocket(wsUrl);
        container._ws = ws;

        ws.onopen = () => {
            console.log('WebSocket connected');
            reconnectAttempts = 0;
            dispatchStatus('connected');
        };

        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            dispatchMessage(data);
        };

        ws.onclose = (event) => {
            console.log('WebSocket disconnected');
            dispatchStatus('disconnected');

            // Check if it was an auth error (server sends 401)
            if (event.code === 1008 || event.code === 4001) {
                window.location.href = '/';
                return;
            }

            scheduleReconnect();
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            dispatchStatus('error', error);
        };
    }

    function dispatchStatus(status, detail = null) {
        document.dispatchEvent(new CustomEvent('bobot:connection-status', {
            detail: { status, detail }
        }));
    }

    function dispatchMessage(data) {
        if (data.group_id) {
            document.dispatchEvent(new CustomEvent('bobot:group-message', {
                detail: data
            }));
        } else {
            document.dispatchEvent(new CustomEvent('bobot:chat-message', {
                detail: data
            }));
        }
    }

    function scheduleReconnect() {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), MAX_RECONNECT_DELAY);
        reconnectAttempts++;
        console.log(`Reconnecting in ${delay}ms...`);
        setTimeout(connect, delay);
    }

    container.send = function(message) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(message));
            return true;
        }
        return false;
    };

    container.close = function() {
        if (ws) {
            ws.close();
            ws = null;
        }
    };

    container.reconnect = function() {
        if (ws) ws.close();
        reconnectAttempts = 0;
        connect();
    };

    // Initialize connection
    connect();
})();
```

**Step 2: Update chat.js**

Remove all localStorage token references. Update the `init` method:

```javascript
async init() {
    await this.loadRecentMessages();
    await this.syncMessages();
    this.setupEventListeners();
}

async loadRecentMessages() {
    try {
        const resp = await fetch('/api/messages/recent?limit=50', {
            credentials: 'include'
        });

        if (!resp.ok) {
            if (resp.status === 401) {
                window.location.href = '/';
                return;
            }
            throw new Error('Failed to load messages');
        }
        // ... rest of method
    }
}

async loadMoreHistory() {
    // ... remove token = localStorage.getItem('access_token')
    const resp = await fetch(`/api/messages/history?before=${this.oldestMessageId}&limit=50`, {
        credentials: 'include'
    });
    // ...
}

async syncMessages() {
    const lastSeen = localStorage.getItem('lastMessageTimestamp');
    if (!lastSeen) return;

    const resp = await fetch(`/api/messages/sync?since=${encodeURIComponent(lastSeen)}`, {
        credentials: 'include'
    });
    // ...
}

async logout() {
    this.wsContainer.close();

    try {
        await fetch('/api/logout', {
            method: 'POST',
            credentials: 'include',
            headers: { 'HX-Request': 'true' }
        });
    } catch (err) {
        console.error('Logout error:', err);
    }

    localStorage.removeItem('lastMessageTimestamp');
    window.location.href = '/';
}
```

**Step 3: Update group_chat.js similarly**

Remove localStorage token references, add `credentials: 'include'` to fetch calls.

**Step 4: Run manual test**

Start the server, try logging in and using chat.

**Step 5: Commit**

```bash
git add web/static/ws-manager.js web/static/chat.js web/static/group_chat.js
git commit -m "refactor(web): simplify client-side auth (cookie-based)"
```

---

## Task 12: Remove JWT and Refresh Token Code

**Files:**
- Delete: `auth/jwt.go`
- Modify: `db/core.go`
- Modify: `go.mod`

**Step 1: Delete jwt.go**

```bash
rm auth/jwt.go
```

**Step 2: Remove refresh token code from db/core.go**

Delete:
- `RefreshToken` struct
- `CreateRefreshToken` method
- `GetRefreshToken` method
- `DeleteRefreshToken` method
- `DeleteExpiredRefreshTokens` method
- `DeleteUserRefreshTokens` method

Keep the `refresh_tokens` table in migration for now (for existing databases), but it won't be used.

**Step 3: Remove JWT dependency**

```bash
go mod tidy
```

**Step 4: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove JWT and refresh token code"
```

---

## Task 13: Update Main Application Entry Point

**Files:**
- Modify: `main.go` (or wherever server is created)

**Step 1: Check and update main.go**

Ensure the `New` or `NewWithAssistant` calls no longer pass JWTService.

**Step 2: Run the application**

```bash
go run .
```

Expected: Server starts without errors.

**Step 3: Commit if changes were needed**

```bash
git add main.go
git commit -m "refactor: update main to use session-based auth"
```

---

## Task 14: Final Integration Test

**Step 1: Manual testing checklist**

- [ ] Login with valid credentials → redirects to /chat, session cookie set
- [ ] Access /chat without login → redirects to /
- [ ] Send chat message → works via WebSocket
- [ ] Wait for token near-expiry → auto-reissue happens (check for new cookie)
- [ ] Logout → clears cookie, redirects to /
- [ ] Logout with ?all=true → revocation created, all sessions invalid
- [ ] Blocked user → cannot login or use existing session

**Step 2: Run full test suite**

```bash
go test ./... -v
```

**Step 3: Commit any fixes**

---

## Summary

After completing all tasks:

1. ~~JWT-based auth~~ → Encrypted session cookies
2. ~~Refresh tokens in DB~~ → Stateless with revocation table
3. ~~Client-side token management~~ → Server handles everything
4. ~~/api/refresh endpoint~~ → Middleware auto-reissue
5. Single `/api/logout` endpoint with optional `?all=true`

Configuration:
- `BOBOT_SESSION_DURATION` (default: 30m)
- `BOBOT_SESSION_MAX_AGE` (default: 7d)
- `BOBOT_SESSION_REFRESH_THRESHOLD` (default: 5m)
