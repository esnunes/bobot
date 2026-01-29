# bobot-web v1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a self-hosted AI assistant with mobile-first chat interface for managing daily family tasks.

**Architecture:** Go HTTP server with WebSocket chat, JWT authentication, SQLite databases, pluggable LLM provider (z.ai), and a tool/skill system for extensibility.

**Tech Stack:** Go 1.25.5, SQLite, vanilla HTML/CSS/JS, WebSocket, JWT, Anthropic-compatible LLM API

---

## Phase 1: Project Foundation

### Task 1.1: Initialize Go Module and Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add required dependencies to go.mod**

```bash
go get github.com/golang-jwt/jwt/v5
go get github.com/gorilla/websocket
go get github.com/mattn/go-sqlite3
go get golang.org/x/crypto/bcrypt
go get github.com/invopop/jsonschema
go get gopkg.in/yaml.v3
```

**Step 2: Verify dependencies installed**

Run: `go mod tidy && cat go.mod`
Expected: All dependencies listed in go.mod

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add project dependencies"
```

---

### Task 1.2: Create Configuration Loader

**Files:**
- Create: `config/config.go`
- Create: `config/config_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./config/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add config/
git commit -m "feat: add configuration loader with env var support"
```

---

### Task 1.3: Create Core Database Schema and Connection

**Files:**
- Create: `db/core.go`
- Create: `db/core_test.go`

**Step 1: Write the failing test**

```go
// db/core_test.go
package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCoreDB_CreatesSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "core.db")

	db, err := NewCoreDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	// Verify tables exist
	tables := []string{"users", "refresh_tokens", "messages"}
	for _, table := range tables {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestCoreDB_CreateUser(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, err := db.CreateUser("testuser", "hashedpass")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.ID == 0 {
		t.Error("expected user ID to be set")
	}
	if user.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", user.Username)
	}
}

func TestCoreDB_GetUserByUsername(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	db.CreateUser("findme", "hashedpass")

	user, err := db.GetUserByUsername("findme")
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if user.Username != "findme" {
		t.Errorf("expected username findme, got %s", user.Username)
	}
}

func TestCoreDB_UserNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	_, err := db.GetUserByUsername("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
// db/core.go
package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type RefreshToken struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Message struct {
	ID        int64
	UserID    int64
	Role      string
	Content   string
	CreatedAt time.Time
}

type CoreDB struct {
	db *sql.DB
}

func NewCoreDB(dbPath string) (*CoreDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}

	coreDB := &CoreDB{db: db}
	if err := coreDB.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return coreDB, nil
}

func (c *CoreDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS refresh_tokens (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := c.db.Exec(schema)
	return err
}

func (c *CoreDB) Close() error {
	return c.db.Close()
}

func (c *CoreDB) CreateUser(username, passwordHash string) (*User, error) {
	result, err := c.db.Exec(
		"INSERT INTO users (username, password_hash) VALUES (?, ?)",
		username, passwordHash,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
	}, nil
}

func (c *CoreDB) GetUserByUsername(username string) (*User, error) {
	var user User
	err := c.db.QueryRow(
		"SELECT id, username, password_hash, created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *CoreDB) GetUserByID(id int64) (*User, error) {
	var user User
	err := c.db.QueryRow(
		"SELECT id, username, password_hash, created_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *CoreDB) UserCount() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/
git commit -m "feat: add core database with user schema"
```

---

### Task 1.4: Add Refresh Token Operations to Core DB

**Files:**
- Modify: `db/core_test.go`
- Modify: `db/core.go`

**Step 1: Write the failing test**

```go
// Add to db/core_test.go

func TestCoreDB_RefreshTokens(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("tokenuser", "hash")

	// Create token
	token, err := db.CreateRefreshToken(user.ID, "token123", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	if token.Token != "token123" {
		t.Errorf("expected token token123, got %s", token.Token)
	}

	// Get token
	found, err := db.GetRefreshToken("token123")
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}
	if found.UserID != user.ID {
		t.Errorf("expected user_id %d, got %d", user.ID, found.UserID)
	}

	// Delete token
	err = db.DeleteRefreshToken("token123")
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	_, err = db.GetRefreshToken("token123")
	if err != ErrNotFound {
		t.Error("expected token to be deleted")
	}
}

func TestCoreDB_DeleteExpiredTokens(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("expireuser", "hash")

	// Create expired token
	db.CreateRefreshToken(user.ID, "expired", time.Now().Add(-1*time.Hour))
	// Create valid token
	db.CreateRefreshToken(user.ID, "valid", time.Now().Add(1*time.Hour))

	deleted, err := db.DeleteExpiredRefreshTokens()
	if err != nil {
		t.Fatalf("failed to delete expired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Valid token should still exist
	_, err = db.GetRefreshToken("valid")
	if err != nil {
		t.Error("valid token should still exist")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/... -v`
Expected: FAIL (methods don't exist)

**Step 3: Write minimal implementation**

```go
// Add to db/core.go

func (c *CoreDB) CreateRefreshToken(userID int64, token string, expiresAt time.Time) (*RefreshToken, error) {
	result, err := c.db.Exec(
		"INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &RefreshToken{
		ID:        id,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

func (c *CoreDB) GetRefreshToken(token string) (*RefreshToken, error) {
	var rt RefreshToken
	err := c.db.QueryRow(
		"SELECT id, user_id, token, expires_at, created_at FROM refresh_tokens WHERE token = ?",
		token,
	).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (c *CoreDB) DeleteRefreshToken(token string) error {
	_, err := c.db.Exec("DELETE FROM refresh_tokens WHERE token = ?", token)
	return err
}

func (c *CoreDB) DeleteExpiredRefreshTokens() (int64, error) {
	result, err := c.db.Exec("DELETE FROM refresh_tokens WHERE expires_at < ?", time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (c *CoreDB) DeleteUserRefreshTokens(userID int64) error {
	_, err := c.db.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/
git commit -m "feat: add refresh token operations to core db"
```

---

### Task 1.5: Add Message Operations to Core DB

**Files:**
- Modify: `db/core_test.go`
- Modify: `db/core.go`

**Step 1: Write the failing test**

```go
// Add to db/core_test.go

func TestCoreDB_Messages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("msguser", "hash")

	// Create messages
	msg1, err := db.CreateMessage(user.ID, "user", "Hello")
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}
	if msg1.Content != "Hello" {
		t.Errorf("expected content Hello, got %s", msg1.Content)
	}

	db.CreateMessage(user.ID, "assistant", "Hi there!")

	// Get messages
	messages, err := db.GetMessages(user.ID, 10)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	// Messages should be in chronological order
	if messages[0].Role != "user" {
		t.Error("first message should be from user")
	}
	if messages[1].Role != "assistant" {
		t.Error("second message should be from assistant")
	}
}

func TestCoreDB_GetMessagesLimit(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("limituser", "hash")

	for i := 0; i < 5; i++ {
		db.CreateMessage(user.ID, "user", "msg")
	}

	messages, _ := db.GetMessages(user.ID, 3)
	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/... -v`
Expected: FAIL (methods don't exist)

**Step 3: Write minimal implementation**

```go
// Add to db/core.go

func (c *CoreDB) CreateMessage(userID int64, role, content string) (*Message, error) {
	result, err := c.db.Exec(
		"INSERT INTO messages (user_id, role, content) VALUES (?, ?, ?)",
		userID, role, content,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:        id,
		UserID:    userID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}, nil
}

func (c *CoreDB) GetMessages(userID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, created_at
		FROM messages
		WHERE user_id = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) GetRecentMessages(userID int64, limit int) ([]Message, error) {
	// Get the most recent N messages, but return in chronological order
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, created_at FROM (
			SELECT id, user_id, role, content, created_at
			FROM messages
			WHERE user_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) ORDER BY created_at ASC
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/
git commit -m "feat: add message operations to core db"
```

---

## Phase 2: Authentication

### Task 2.1: Create JWT Token Service

**Files:**
- Create: `auth/jwt.go`
- Create: `auth/jwt_test.go`

**Step 1: Write the failing test**

```go
// auth/jwt_test.go
package auth

import (
	"testing"
	"time"
)

func TestJWTService_GenerateAccessToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	token, err := svc.GenerateAccessToken(123)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestJWTService_ValidateAccessToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	token, _ := svc.GenerateAccessToken(456)

	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}
	if claims.UserID != 456 {
		t.Errorf("expected user_id 456, got %d", claims.UserID)
	}
}

func TestJWTService_ExpiredToken(t *testing.T) {
	svc := &JWTService{
		secret:      []byte("test-secret-key-32-chars-min!!"),
		accessTTL:   -1 * time.Hour, // Already expired
		refreshTTL:  7 * 24 * time.Hour,
	}

	token, _ := svc.GenerateAccessToken(789)

	_, err := svc.ValidateAccessToken(token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestJWTService_InvalidToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	_, err := svc.ValidateAccessToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestJWTService_GenerateRefreshToken(t *testing.T) {
	svc := NewJWTService("test-secret")

	token := svc.GenerateRefreshToken()
	if len(token) < 32 {
		t.Error("refresh token should be at least 32 chars")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./auth/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
// auth/jwt.go
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewJWTService(secret string) *JWTService {
	return &JWTService{
		secret:     []byte(secret),
		accessTTL:  15 * time.Minute,
		refreshTTL: 7 * 24 * time.Hour,
	}
}

func (s *JWTService) GenerateAccessToken(userID int64) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *JWTService) ValidateAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return s.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (s *JWTService) GenerateRefreshToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (s *JWTService) RefreshTTL() time.Duration {
	return s.refreshTTL
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./auth/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add auth/
git commit -m "feat: add JWT token service"
```

---

### Task 2.2: Create Password Hashing Service

**Files:**
- Create: `auth/password.go`
- Create: `auth/password_test.go`

**Step 1: Write the failing test**

```go
// auth/password_test.go
package auth

import "testing"

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if hash == "mypassword" {
		t.Error("hash should not equal plaintext")
	}
	if len(hash) < 50 {
		t.Error("hash seems too short for bcrypt")
	}
}

func TestCheckPassword_Valid(t *testing.T) {
	hash, _ := HashPassword("correctpassword")

	if !CheckPassword("correctpassword", hash) {
		t.Error("expected password to match")
	}
}

func TestCheckPassword_Invalid(t *testing.T) {
	hash, _ := HashPassword("correctpassword")

	if CheckPassword("wrongpassword", hash) {
		t.Error("expected password not to match")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./auth/... -v -run TestHashPassword`
Expected: FAIL (function doesn't exist)

**Step 3: Write minimal implementation**

```go
// auth/password.go
package auth

import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./auth/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add auth/
git commit -m "feat: add password hashing with bcrypt"
```

---

### Task 2.3: Create HTTP Server Foundation

**Files:**
- Create: `server/server.go`
- Create: `server/server_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./server/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
// server/server.go
package server

import (
	"encoding/json"
	"net/http"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

type Server struct {
	cfg    *config.Config
	db     *db.CoreDB
	jwt    *auth.JWTService
	router *http.ServeMux
}

func New(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService) *Server {
	s := &Server{
		cfg:    cfg,
		db:     coreDB,
		jwt:    jwt,
		router: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.router.HandleFunc("GET /health", s.handleHealth)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/
git commit -m "feat: add HTTP server foundation with health check"
```

---

### Task 2.4: Implement Login Endpoint

**Files:**
- Modify: `server/server_test.go`
- Create: `server/auth.go`
- Modify: `server/server.go`

**Step 1: Write the failing test**

```go
// Add to server/server_test.go

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
```

**Step 2: Run test to verify it fails**

Run: `go test ./server/... -v -run TestServer_Login`
Expected: FAIL (endpoint doesn't exist)

**Step 3: Write minimal implementation**

```go
// server/auth.go
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

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

	accessToken, err := s.jwt.GenerateAccessToken(user.ID)
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

Update routes in server.go:

```go
// Update routes() in server/server.go
func (s *Server) routes() {
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/login", s.handleLogin)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/
git commit -m "feat: add login endpoint with JWT tokens"
```

---

### Task 2.5: Implement Token Refresh Endpoint

**Files:**
- Modify: `server/server_test.go`
- Modify: `server/auth.go`
- Modify: `server/server.go`

**Step 1: Write the failing test**

```go
// Add to server/server_test.go

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
```

**Step 2: Run test to verify it fails**

Run: `go test ./server/... -v -run TestServer_Refresh`
Expected: FAIL (endpoint doesn't exist)

**Step 3: Write minimal implementation**

```go
// Add to server/auth.go

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

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

	accessToken, err := s.jwt.GenerateAccessToken(token.UserID)
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

Update routes:

```go
// Update routes() in server/server.go
func (s *Server) routes() {
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/login", s.handleLogin)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/
git commit -m "feat: add token refresh endpoint"
```

---

### Task 2.6: Implement Logout Endpoint

**Files:**
- Modify: `server/server_test.go`
- Modify: `server/auth.go`
- Modify: `server/server.go`

**Step 1: Write the failing test**

```go
// Add to server/server_test.go

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
```

**Step 2: Run test to verify it fails**

Run: `go test ./server/... -v -run TestServer_Logout`
Expected: FAIL (endpoint doesn't exist)

**Step 3: Write minimal implementation**

```go
// Add to server/auth.go

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	s.db.DeleteRefreshToken(req.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

Update routes:

```go
// Update routes() in server/server.go
func (s *Server) routes() {
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/login", s.handleLogin)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
	s.router.HandleFunc("POST /api/logout", s.handleLogout)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/
git commit -m "feat: add logout endpoint"
```

---

### Task 2.7: Create Auth Middleware

**Files:**
- Create: `server/middleware.go`
- Create: `server/middleware_test.go`

**Step 1: Write the failing test**

```go
// server/middleware_test.go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func TestAuthMiddleware_ValidToken(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")
	jwtSvc := auth.NewJWTService("test-secret-32-chars-minimum!!")

	user, _ := coreDB.CreateUser("testuser", "hash")
	token, _ := jwtSvc.GenerateAccessToken(user.ID)

	mw := NewAuthMiddleware(jwtSvc, coreDB)

	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != user.ID {
			t.Errorf("expected user_id %d, got %d", user.ID, userID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")
	jwtSvc := auth.NewJWTService("test-secret-32-chars-minimum!!")

	mw := NewAuthMiddleware(jwtSvc, coreDB)

	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")
	jwtSvc := auth.NewJWTService("test-secret-32-chars-minimum!!")

	mw := NewAuthMiddleware(jwtSvc, coreDB)

	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server/... -v -run TestAuthMiddleware`
Expected: FAIL (middleware doesn't exist)

**Step 3: Write minimal implementation**

```go
// server/middleware.go
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type contextKey string

const userIDKey contextKey = "user_id"

type AuthMiddleware struct {
	jwt *auth.JWTService
	db  *db.CoreDB
}

func NewAuthMiddleware(jwt *auth.JWTService, db *db.CoreDB) *AuthMiddleware {
	return &AuthMiddleware{jwt: jwt, db: db}
}

func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		claims, err := m.jwt.ValidateAccessToken(parts[1])
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserID(ctx context.Context) int64 {
	if id, ok := ctx.Value(userIDKey).(int64); ok {
		return id
	}
	return 0
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/
git commit -m "feat: add authentication middleware"
```

---

## Phase 3: LLM Provider

### Task 3.1: Create LLM Provider Interface and Types

**Files:**
- Create: `llm/provider.go`

**Step 1: Create the interface (no test needed for interface definition)**

```go
// llm/provider.go
package llm

import "context"

type Provider interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

type ChatRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []Tool
}

type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"`
	ToolUseID  string      `json:"tool_use_id,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
	StopType  string
}

type ToolCall struct {
	ID    string
	Name  string
	Input map[string]interface{}
}
```

**Step 2: Commit**

```bash
git add llm/
git commit -m "feat: add LLM provider interface and types"
```

---

### Task 3.2: Implement Anthropic-Compatible Client

**Files:**
- Create: `llm/anthropic.go`
- Create: `llm/anthropic_test.go`

**Step 1: Write the failing test**

```go
// llm/anthropic_test.go
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicClient_Chat_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("expected x-api-key header")
		}

		resp := map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello!"},
			},
			"stop_reason": "end_turn",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "test-key", "test-model")

	resp, err := client.Chat(context.Background(), &ChatRequest{
		SystemPrompt: "You are helpful",
		Messages:     []Message{{Role: "user", Content: "Hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("expected 'Hello!', got '%s'", resp.Content)
	}
}

func TestAnthropicClient_Chat_ToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "tool_use",
					"id":   "call_123",
					"name": "task",
					"input": map[string]interface{}{
						"command": "list",
						"project": "groceries",
					},
				},
			},
			"stop_reason": "tool_use",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "test-key", "test-model")

	resp, err := client.Chat(context.Background(), &ChatRequest{
		SystemPrompt: "You are helpful",
		Messages:     []Message{{Role: "user", Content: "List groceries"}},
		Tools: []Tool{{
			Name:        "task",
			Description: "Manage tasks",
			InputSchema: map[string]interface{}{"type": "object"},
		}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "task" {
		t.Errorf("expected tool name 'task', got '%s'", resp.ToolCalls[0].Name)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./llm/... -v`
Expected: FAIL (client doesn't exist)

**Step 3: Write minimal implementation**

```go
// llm/anthropic.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AnthropicClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string) *AnthropicClient {
	return &AnthropicClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []anthropicMsg   `json:"messages"`
	Tools     []anthropicTool  `json:"tools,omitempty"`
}

type anthropicMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
}

type anthropicContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

func (c *AnthropicClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	msgs := make([]anthropicMsg, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = anthropicMsg{Role: m.Role, Content: m.Content}
	}

	tools := make([]anthropicTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	apiReq := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    req.SystemPrompt,
		Messages:  msgs,
	}
	if len(tools) > 0 {
		apiReq.Tools = tools
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	result := &ChatResponse{
		StopType: apiResp.StopReason,
	}

	for _, content := range apiResp.Content {
		switch content.Type {
		case "text":
			result.Content += content.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:    content.ID,
				Name:  content.Name,
				Input: content.Input,
			})
		}
	}

	return result, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./llm/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add llm/
git commit -m "feat: add Anthropic-compatible LLM client"
```

---

## Phase 4: Tool System

### Task 4.1: Create Tool Interface and Registry

**Files:**
- Create: `tools/registry.go`
- Create: `tools/registry_test.go`

**Step 1: Write the failing test**

```go
// tools/registry_test.go
package tools

import (
	"context"
	"testing"
)

type mockTool struct{}

func (m *mockTool) Name() string        { return "mock" }
func (m *mockTool) Description() string { return "A mock tool" }
func (m *mockTool) Schema() interface{} { return map[string]string{"type": "object"} }
func (m *mockTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	return "executed", nil
}

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	mock := &mockTool{}

	reg.Register(mock)

	if len(reg.List()) != 1 {
		t.Errorf("expected 1 tool, got %d", len(reg.List()))
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{})

	tool, ok := reg.Get("mock")
	if !ok {
		t.Fatal("expected to find mock tool")
	}
	if tool.Name() != "mock" {
		t.Errorf("expected name mock, got %s", tool.Name())
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry()

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("expected not to find tool")
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{})

	result, err := reg.Execute(context.Background(), "mock", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "executed" {
		t.Errorf("expected 'executed', got '%s'", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./tools/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
// tools/registry.go
package tools

import (
	"context"
	"fmt"

	"github.com/esnunes/bobot/llm"
)

type Tool interface {
	Name() string
	Description() string
	Schema() interface{}
	Execute(ctx context.Context, input map[string]interface{}) (string, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	return result
}

func (r *Registry) Execute(ctx context.Context, name string, input map[string]interface{}) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Execute(ctx, input)
}

func (r *Registry) ToLLMTools() []llm.Tool {
	result := make([]llm.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, llm.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		})
	}
	return result
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./tools/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tools/
git commit -m "feat: add tool interface and registry"
```

---

### Task 4.2: Create Task Tool Database

**Files:**
- Create: `tools/task/db.go`
- Create: `tools/task/db_test.go`

**Step 1: Write the failing test**

```go
// tools/task/db_test.go
package task

import (
	"path/filepath"
	"testing"
)

func TestTaskDB_CreateProject(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	proj, err := db.CreateProject(1, "groceries")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	if proj.Name != "groceries" {
		t.Errorf("expected name groceries, got %s", proj.Name)
	}
}

func TestTaskDB_GetOrCreateProject(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	p1, _ := db.GetOrCreateProject(1, "groceries")
	p2, _ := db.GetOrCreateProject(1, "groceries")

	if p1.ID != p2.ID {
		t.Error("expected same project ID")
	}
}

func TestTaskDB_CreateTask(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	task, err := db.CreateTask(proj.ID, "milk")
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if task.Name != "milk" {
		t.Errorf("expected name milk, got %s", task.Name)
	}
	if task.Status != "pending" {
		t.Errorf("expected status pending, got %s", task.Status)
	}
}

func TestTaskDB_ListTasks(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	db.CreateTask(proj.ID, "milk")
	db.CreateTask(proj.ID, "eggs")

	tasks, err := db.ListTasks(proj.ID, "")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestTaskDB_ListTasks_FilterByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	db.CreateTask(proj.ID, "milk")
	task2, _ := db.CreateTask(proj.ID, "eggs")
	db.UpdateTaskStatus(task2.ID, "done")

	pending, _ := db.ListTasks(proj.ID, "pending")
	if len(pending) != 1 {
		t.Errorf("expected 1 pending task, got %d", len(pending))
	}

	done, _ := db.ListTasks(proj.ID, "done")
	if len(done) != 1 {
		t.Errorf("expected 1 done task, got %d", len(done))
	}
}

func TestTaskDB_UpdateTaskStatus(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	task, _ := db.CreateTask(proj.ID, "milk")

	err := db.UpdateTaskStatus(task.ID, "done")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	updated, _ := db.GetTask(task.ID)
	if updated.Status != "done" {
		t.Errorf("expected status done, got %s", updated.Status)
	}
}

func TestTaskDB_DeleteTask(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	task, _ := db.CreateTask(proj.ID, "milk")

	err := db.DeleteTask(task.ID)
	if err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	_, err = db.GetTask(task.ID)
	if err == nil {
		t.Error("expected error getting deleted task")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./tools/task/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
// tools/task/db.go
package task

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var ErrNotFound = errors.New("not found")

type Project struct {
	ID        int64
	UserID    int64
	Name      string
	CreatedAt time.Time
}

type Task struct {
	ID        int64
	ProjectID int64
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TaskDB struct {
	db *sql.DB
}

func NewTaskDB(dbPath string) (*TaskDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}

	taskDB := &TaskDB{db: db}
	if err := taskDB.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return taskDB, nil
}

func (t *TaskDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, name)
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := t.db.Exec(schema)
	return err
}

func (t *TaskDB) Close() error {
	return t.db.Close()
}

func (t *TaskDB) CreateProject(userID int64, name string) (*Project, error) {
	result, err := t.db.Exec(
		"INSERT INTO projects (user_id, name) VALUES (?, ?)",
		userID, name,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Project{ID: id, UserID: userID, Name: name, CreatedAt: time.Now()}, nil
}

func (t *TaskDB) GetProject(userID int64, name string) (*Project, error) {
	var p Project
	err := t.db.QueryRow(
		"SELECT id, user_id, name, created_at FROM projects WHERE user_id = ? AND name = ?",
		userID, name,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &p, err
}

func (t *TaskDB) GetOrCreateProject(userID int64, name string) (*Project, error) {
	p, err := t.GetProject(userID, name)
	if err == nil {
		return p, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	return t.CreateProject(userID, name)
}

func (t *TaskDB) CreateTask(projectID int64, name string) (*Task, error) {
	result, err := t.db.Exec(
		"INSERT INTO tasks (project_id, name) VALUES (?, ?)",
		projectID, name,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Task{
		ID:        id,
		ProjectID: projectID,
		Name:      name,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (t *TaskDB) GetTask(id int64) (*Task, error) {
	var task Task
	err := t.db.QueryRow(
		"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE id = ?",
		id,
	).Scan(&task.ID, &task.ProjectID, &task.Name, &task.Status, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &task, err
}

func (t *TaskDB) ListTasks(projectID int64, status string) ([]Task, error) {
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = t.db.Query(
			"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE project_id = ? ORDER BY created_at",
			projectID,
		)
	} else {
		rows, err = t.db.Query(
			"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE project_id = ? AND status = ? ORDER BY created_at",
			projectID, status,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Name, &task.Status, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (t *TaskDB) UpdateTaskStatus(id int64, status string) error {
	_, err := t.db.Exec(
		"UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?",
		status, time.Now(), id,
	)
	return err
}

func (t *TaskDB) DeleteTask(id int64) error {
	_, err := t.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	return err
}

func (t *TaskDB) FindTaskByName(projectID int64, name string) (*Task, error) {
	var task Task
	err := t.db.QueryRow(
		"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE project_id = ? AND name = ?",
		projectID, name,
	).Scan(&task.ID, &task.ProjectID, &task.Name, &task.Status, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &task, err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./tools/task/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tools/
git commit -m "feat: add task tool database"
```

---

### Task 4.3: Implement Task Tool

**Files:**
- Create: `tools/task/task.go`
- Create: `tools/task/task_test.go`

**Step 1: Write the failing test**

```go
// tools/task/task_test.go
package task

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestTool(t *testing.T) *TaskTool {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	return NewTaskTool(db)
}

func TestTaskTool_Name(t *testing.T) {
	tool := setupTestTool(t)
	if tool.Name() != "task" {
		t.Errorf("expected name 'task', got '%s'", tool.Name())
	}
}

func TestTaskTool_Create(t *testing.T) {
	tool := setupTestTool(t)

	ctx := context.WithValue(context.Background(), "user_id", int64(1))
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "milk") {
		t.Errorf("expected result to contain 'milk', got: %s", result)
	}
}

func TestTaskTool_List(t *testing.T) {
	tool := setupTestTool(t)

	ctx := context.WithValue(context.Background(), "user_id", int64(1))
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "eggs",
	})

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "list",
		"project": "groceries",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "milk") || !strings.Contains(result, "eggs") {
		t.Errorf("expected result to contain milk and eggs, got: %s", result)
	}
}

func TestTaskTool_Update(t *testing.T) {
	tool := setupTestTool(t)

	ctx := context.WithValue(context.Background(), "user_id", int64(1))
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "update",
		"project": "groceries",
		"title":   "milk",
		"status":  "done",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "done") {
		t.Errorf("expected result to indicate done status, got: %s", result)
	}
}

func TestTaskTool_Delete(t *testing.T) {
	tool := setupTestTool(t)

	ctx := context.WithValue(context.Background(), "user_id", int64(1))
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "delete",
		"project": "groceries",
		"title":   "milk",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected result to indicate deletion, got: %s", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./tools/task/... -v -run TestTaskTool`
Expected: FAIL (TaskTool doesn't exist)

**Step 3: Write minimal implementation**

```go
// tools/task/task.go
package task

import (
	"context"
	"fmt"
	"strings"
)

type TaskTool struct {
	db *TaskDB
}

func NewTaskTool(db *TaskDB) *TaskTool {
	return &TaskTool{db: db}
}

func (t *TaskTool) Name() string {
	return "task"
}

func (t *TaskTool) Description() string {
	return "Manage tasks within projects"
}

func (t *TaskTool) Schema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"create", "list", "update", "delete"},
				"description": "The operation to perform",
			},
			"project": map[string]interface{}{
				"type":        "string",
				"description": "Project name (e.g., 'groceries')",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Task title",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"pending", "done"},
				"description": "Task status",
			},
		},
		"required": []string{"command", "project"},
	}
}

func (t *TaskTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	userID, ok := ctx.Value("user_id").(int64)
	if !ok {
		return "", fmt.Errorf("user_id not found in context")
	}

	command, _ := input["command"].(string)
	projectName, _ := input["project"].(string)
	title, _ := input["title"].(string)
	status, _ := input["status"].(string)

	project, err := t.db.GetOrCreateProject(userID, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to get/create project: %w", err)
	}

	switch command {
	case "create":
		return t.create(project.ID, title)
	case "list":
		return t.list(project.ID, status)
	case "update":
		return t.update(project.ID, title, status)
	case "delete":
		return t.delete(project.ID, title)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *TaskTool) create(projectID int64, title string) (string, error) {
	task, err := t.db.CreateTask(projectID, title)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Created task: %s (id: %d)", task.Name, task.ID), nil
}

func (t *TaskTool) list(projectID int64, status string) (string, error) {
	tasks, err := t.db.ListTasks(projectID, status)
	if err != nil {
		return "", err
	}

	if len(tasks) == 0 {
		return "No tasks found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Tasks:\n")
	for _, task := range tasks {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", task.Status, task.Name))
	}
	return sb.String(), nil
}

func (t *TaskTool) update(projectID int64, title, status string) (string, error) {
	task, err := t.db.FindTaskByName(projectID, title)
	if err != nil {
		return "", fmt.Errorf("task not found: %s", title)
	}

	if err := t.db.UpdateTaskStatus(task.ID, status); err != nil {
		return "", err
	}

	return fmt.Sprintf("Updated task '%s' to status: %s", title, status), nil
}

func (t *TaskTool) delete(projectID int64, title string) (string, error) {
	task, err := t.db.FindTaskByName(projectID, title)
	if err != nil {
		return "", fmt.Errorf("task not found: %s", title)
	}

	if err := t.db.DeleteTask(task.ID); err != nil {
		return "", err
	}

	return fmt.Sprintf("Task '%s' deleted.", title), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./tools/task/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tools/
git commit -m "feat: implement task tool with CRUD operations"
```

---

## Phase 5: Assistant Engine

### Task 5.1: Create Skill Loader

**Files:**
- Create: `assistant/skills.go`
- Create: `assistant/skills_test.go`
- Create: `skills/groceries.md` (test fixture)

**Step 1: Write the failing test**

```go
// assistant/skills_test.go
package assistant

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkills(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	os.MkdirAll(skillsDir, 0755)

	// Create test skill
	skillContent := `---
name: groceries
description: Manage grocery shopping lists
---
When the user wants to manage their grocery list, use the task tool.
`
	os.WriteFile(filepath.Join(skillsDir, "groceries.md"), []byte(skillContent), 0644)

	skills, err := LoadSkills(skillsDir)
	if err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	skill := skills[0]
	if skill.Name != "groceries" {
		t.Errorf("expected name groceries, got %s", skill.Name)
	}
	if skill.Description != "Manage grocery shopping lists" {
		t.Errorf("unexpected description: %s", skill.Description)
	}
	if skill.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestLoadSkills_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	os.MkdirAll(skillsDir, 0755)

	skills, err := LoadSkills(skillsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./assistant/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
// assistant/skills.go
package assistant

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Content     string `yaml:"-"`
}

func LoadSkills(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		skill, err := loadSkillFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		skills = append(skills, *skill)
	}

	return skills, nil
}

func loadSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// Parse YAML frontmatter
	if !strings.HasPrefix(content, "---") {
		return &Skill{Content: content}, nil
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) != 2 {
		return &Skill{Content: content}, nil
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(parts[0]), &skill); err != nil {
		return nil, err
	}

	skill.Content = strings.TrimSpace(parts[1])
	return &skill, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./assistant/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add assistant/
git commit -m "feat: add skill loader with YAML frontmatter parsing"
```

---

### Task 5.2: Create System Prompt Builder

**Files:**
- Create: `assistant/prompt.go`
- Create: `assistant/prompt_test.go`

**Step 1: Write the failing test**

```go
// assistant/prompt_test.go
package assistant

import (
	"strings"
	"testing"

	"github.com/esnunes/bobot/llm"
)

func TestBuildSystemPrompt(t *testing.T) {
	skills := []Skill{
		{Name: "groceries", Description: "Manage groceries", Content: "Use task tool for groceries."},
	}
	tools := []llm.Tool{
		{Name: "task", Description: "Manage tasks"},
	}

	prompt := BuildSystemPrompt(skills, tools)

	if !strings.Contains(prompt, "groceries") {
		t.Error("expected prompt to contain skill name")
	}
	if !strings.Contains(prompt, "task") {
		t.Error("expected prompt to contain tool name")
	}
	if !strings.Contains(prompt, "Use task tool for groceries") {
		t.Error("expected prompt to contain skill content")
	}
}

func TestBuildSystemPrompt_NoSkills(t *testing.T) {
	tools := []llm.Tool{
		{Name: "task", Description: "Manage tasks"},
	}

	prompt := BuildSystemPrompt(nil, tools)

	if prompt == "" {
		t.Error("expected non-empty prompt even without skills")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./assistant/... -v -run TestBuildSystemPrompt`
Expected: FAIL (function doesn't exist)

**Step 3: Write minimal implementation**

```go
// assistant/prompt.go
package assistant

import (
	"fmt"
	"strings"

	"github.com/esnunes/bobot/llm"
)

const basePrompt = `You are bobot, a helpful AI assistant for managing daily family tasks. You are friendly, concise, and efficient.

Respond adaptively:
- Be terse for simple, clear requests (e.g., "Added milk")
- Be conversational when clarification is needed (e.g., "Added milk. Whole or skim?")

Always use available tools when appropriate to help users manage their tasks.`

func BuildSystemPrompt(skills []Skill, tools []llm.Tool) string {
	var sb strings.Builder

	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")

	if len(skills) > 0 {
		sb.WriteString("## Skills\n\n")
		for _, skill := range skills {
			sb.WriteString(fmt.Sprintf("### %s\n", skill.Name))
			if skill.Description != "" {
				sb.WriteString(fmt.Sprintf("*%s*\n\n", skill.Description))
			}
			sb.WriteString(skill.Content)
			sb.WriteString("\n\n")
		}
	}

	if len(tools) > 0 {
		sb.WriteString("## Available Tools\n\n")
		for _, tool := range tools {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description))
		}
	}

	return sb.String()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./assistant/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add assistant/
git commit -m "feat: add system prompt builder"
```

---

### Task 5.3: Create Assistant Engine

**Files:**
- Create: `assistant/engine.go`
- Create: `assistant/engine_test.go`

**Step 1: Write the failing test**

```go
// assistant/engine_test.go
package assistant

import (
	"context"
	"testing"

	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

type mockLLM struct {
	responses []*llm.ChatResponse
	callCount int
}

func (m *mockLLM) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.callCount >= len(m.responses) {
		return &llm.ChatResponse{Content: "default response"}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

type mockTool struct {
	result string
}

func (m *mockTool) Name() string                { return "task" }
func (m *mockTool) Description() string         { return "Manage tasks" }
func (m *mockTool) Schema() interface{}         { return map[string]interface{}{"type": "object"} }
func (m *mockTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	return m.result, nil
}

func TestEngine_Chat_SimpleResponse(t *testing.T) {
	mockProvider := &mockLLM{
		responses: []*llm.ChatResponse{
			{Content: "Hello!", StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	engine := NewEngine(mockProvider, registry, nil)

	result, err := engine.Chat(context.Background(), 1, "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Errorf("expected 'Hello!', got '%s'", result)
	}
}

func TestEngine_Chat_WithToolUse(t *testing.T) {
	mockProvider := &mockLLM{
		responses: []*llm.ChatResponse{
			{
				StopType: "tool_use",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "task", Input: map[string]interface{}{"command": "list"}},
				},
			},
			{Content: "Here are your tasks: milk, eggs", StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&mockTool{result: "Tasks: milk, eggs"})

	engine := NewEngine(mockProvider, registry, nil)

	result, err := engine.Chat(context.Background(), 1, "What's on my list?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Here are your tasks: milk, eggs" {
		t.Errorf("unexpected result: %s", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./assistant/... -v -run TestEngine`
Expected: FAIL (Engine doesn't exist)

**Step 3: Write minimal implementation**

```go
// assistant/engine.go
package assistant

import (
	"context"
	"fmt"

	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

type Engine struct {
	provider llm.Provider
	registry *tools.Registry
	skills   []Skill
}

func NewEngine(provider llm.Provider, registry *tools.Registry, skills []Skill) *Engine {
	return &Engine{
		provider: provider,
		registry: registry,
		skills:   skills,
	}
}

func (e *Engine) Chat(ctx context.Context, userID int64, message string) (string, error) {
	// Build system prompt
	systemPrompt := BuildSystemPrompt(e.skills, e.registry.ToLLMTools())

	// Add user_id to context for tools
	ctx = context.WithValue(ctx, "user_id", userID)

	// Start with user message
	messages := []llm.Message{
		{Role: "user", Content: message},
	}

	// Loop for tool use
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		resp, err := e.provider.Chat(ctx, &llm.ChatRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        e.registry.ToLLMTools(),
		})
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		// If no tool calls, return the response
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Build assistant message with tool use
		toolUseContent := make([]map[string]interface{}, 0)
		for _, tc := range resp.ToolCalls {
			toolUseContent = append(toolUseContent, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Name,
				"input": tc.Input,
			})
		}
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: toolUseContent,
		})

		// Execute tools and add results
		toolResults := make([]map[string]interface{}, 0)
		for _, tc := range resp.ToolCalls {
			result, err := e.registry.Execute(ctx, tc.Name, tc.Input)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}
			toolResults = append(toolResults, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": tc.ID,
				"content":     result,
			})
		}
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	return "", fmt.Errorf("max iterations reached")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./assistant/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add assistant/
git commit -m "feat: add assistant engine with tool execution loop"
```

---

## Phase 6: WebSocket Chat

### Task 6.1: Create WebSocket Chat Handler

**Files:**
- Create: `server/chat.go`
- Create: `server/chat_test.go`

**Step 1: Write the failing test**

```go
// server/chat_test.go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

type mockLLMProvider struct{}

func (m *mockLLMProvider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: "Hello from assistant!", StopType: "end_turn"}, nil
}

func setupChatTestServer(t *testing.T) (*Server, string) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")
	jwtSvc := auth.NewJWTService("test-secret-32-chars-minimum!!")

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: 8080},
	}

	registry := tools.NewRegistry()
	engine := assistant.NewEngine(&mockLLMProvider{}, registry, nil)

	srv := NewWithAssistant(cfg, coreDB, jwtSvc, engine)

	// Create test user and get token
	hash, _ := auth.HashPassword("testpass")
	user, _ := coreDB.CreateUser("testuser", hash)
	token, _ := jwtSvc.GenerateAccessToken(user.ID)

	return srv, token
}

func TestChatWebSocket_Connect(t *testing.T) {
	srv, token := setupChatTestServer(t)

	server := httptest.NewServer(srv)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/chat?token=" + token

	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("expected 101, got %d", resp.StatusCode)
	}
}

func TestChatWebSocket_SendMessage(t *testing.T) {
	srv, token := setupChatTestServer(t)

	server := httptest.NewServer(srv)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/chat?token=" + token

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send message
	err = conn.WriteJSON(map[string]string{"content": "Hello"})
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp map[string]string
	err = conn.ReadJSON(&resp)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if resp["content"] != "Hello from assistant!" {
		t.Errorf("unexpected response: %s", resp["content"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server/... -v -run TestChatWebSocket`
Expected: FAIL (handler doesn't exist)

**Step 3: Write minimal implementation**

```go
// server/chat.go
package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type chatMessage struct {
	Content string `json:"content"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Get token from query param
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := s.jwt.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Handle messages
	for {
		var msg chatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		// Save user message
		s.db.CreateMessage(claims.UserID, "user", msg.Content)

		// Get assistant response
		response, err := s.engine.Chat(r.Context(), claims.UserID, msg.Content)
		if err != nil {
			log.Printf("assistant error: %v", err)
			response = "Sorry, I encountered an error. Please try again."
		}

		// Save assistant message
		s.db.CreateMessage(claims.UserID, "assistant", response)

		// Send response
		if err := conn.WriteJSON(chatMessage{Content: response}); err != nil {
			log.Printf("websocket write error: %v", err)
			break
		}
	}
}
```

Update server.go to include assistant:

```go
// server/server.go - update struct and constructor
package server

import (
	"encoding/json"
	"net/http"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

type Server struct {
	cfg    *config.Config
	db     *db.CoreDB
	jwt    *auth.JWTService
	engine *assistant.Engine
	router *http.ServeMux
}

func New(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService) *Server {
	return NewWithAssistant(cfg, coreDB, jwt, nil)
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService, engine *assistant.Engine) *Server {
	s := &Server{
		cfg:    cfg,
		db:     coreDB,
		jwt:    jwt,
		engine: engine,
		router: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/login", s.handleLogin)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
	s.router.HandleFunc("POST /api/logout", s.handleLogout)
	s.router.HandleFunc("GET /ws/chat", s.handleChat)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/
git commit -m "feat: add WebSocket chat handler"
```

---

## Phase 7: Web UI

### Task 7.1: Create HTML Templates

**Files:**
- Create: `web/templates/layout.html`
- Create: `web/templates/login.html`
- Create: `web/templates/chat.html`

**Step 1: Create base layout template**

```html
<!-- web/templates/layout.html -->
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>{{.Title}} - bobot</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    {{template "content" .}}
</body>
</html>
```

**Step 2: Create login template**

```html
<!-- web/templates/login.html -->
{{define "content"}}
<div class="login-container">
    <h1>bobot</h1>
    <form id="login-form" class="login-form">
        <input type="text" name="username" placeholder="Username" required autofocus>
        <input type="password" name="password" placeholder="Password" required>
        <button type="submit">Login</button>
        <p id="error-message" class="error hidden"></p>
    </form>
</div>
<script>
document.getElementById('login-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const form = e.target;
    const errorEl = document.getElementById('error-message');

    try {
        const resp = await fetch('/api/login', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                username: form.username.value,
                password: form.password.value
            })
        });

        if (!resp.ok) {
            throw new Error('Invalid credentials');
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

**Step 3: Create chat template**

```html
<!-- web/templates/chat.html -->
{{define "content"}}
<div class="chat-container">
    <header class="chat-header">
        <h1>bobot</h1>
        <button id="menu-btn" class="menu-btn">☰</button>
    </header>

    <main class="chat-messages" id="messages">
        <!-- Messages will be inserted here -->
    </main>

    <footer class="chat-input">
        <form id="chat-form">
            <input type="text" id="message-input" placeholder="Type a message..." autocomplete="off">
            <button type="submit">➤</button>
        </form>
    </footer>
</div>

<div id="menu-overlay" class="menu-overlay hidden">
    <div class="menu">
        <button id="logout-btn">Logout</button>
    </div>
</div>

<script src="/static/chat.js"></script>
{{end}}
```

**Step 4: Commit**

```bash
mkdir -p web/templates
git add web/templates/
git commit -m "feat: add HTML templates for login and chat"
```

---

### Task 7.2: Create CSS Styles

**Files:**
- Create: `web/static/style.css`

**Step 1: Create mobile-first styles**

```css
/* web/static/style.css */
* {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
}

:root {
    --primary-color: #007AFF;
    --bg-color: #f5f5f5;
    --chat-bg: #ffffff;
    --user-msg-bg: #007AFF;
    --user-msg-color: #ffffff;
    --assistant-msg-bg: #e9e9eb;
    --assistant-msg-color: #000000;
    --border-color: #e0e0e0;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background-color: var(--bg-color);
    height: 100vh;
    overflow: hidden;
}

/* Login Page */
.login-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100vh;
    padding: 20px;
}

.login-container h1 {
    font-size: 2.5rem;
    margin-bottom: 30px;
    color: var(--primary-color);
}

.login-form {
    width: 100%;
    max-width: 300px;
    display: flex;
    flex-direction: column;
    gap: 15px;
}

.login-form input {
    padding: 15px;
    border: 1px solid var(--border-color);
    border-radius: 8px;
    font-size: 16px;
}

.login-form button {
    padding: 15px;
    background-color: var(--primary-color);
    color: white;
    border: none;
    border-radius: 8px;
    font-size: 16px;
    cursor: pointer;
}

.login-form button:hover {
    opacity: 0.9;
}

.error {
    color: #ff3b30;
    text-align: center;
    font-size: 14px;
}

.hidden {
    display: none !important;
}

/* Chat Page */
.chat-container {
    display: flex;
    flex-direction: column;
    height: 100vh;
    background-color: var(--chat-bg);
}

.chat-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 15px 20px;
    background-color: var(--primary-color);
    color: white;
    position: sticky;
    top: 0;
    z-index: 10;
}

.chat-header h1 {
    font-size: 1.2rem;
    font-weight: 600;
}

.menu-btn {
    background: none;
    border: none;
    color: white;
    font-size: 1.5rem;
    cursor: pointer;
}

.chat-messages {
    flex: 1;
    overflow-y: auto;
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 10px;
}

.message {
    max-width: 80%;
    padding: 12px 16px;
    border-radius: 18px;
    word-wrap: break-word;
    line-height: 1.4;
}

.message.user {
    align-self: flex-end;
    background-color: var(--user-msg-bg);
    color: var(--user-msg-color);
    border-bottom-right-radius: 4px;
}

.message.assistant {
    align-self: flex-start;
    background-color: var(--assistant-msg-bg);
    color: var(--assistant-msg-color);
    border-bottom-left-radius: 4px;
}

.chat-input {
    padding: 10px 15px;
    background-color: var(--chat-bg);
    border-top: 1px solid var(--border-color);
    position: sticky;
    bottom: 0;
}

.chat-input form {
    display: flex;
    gap: 10px;
}

.chat-input input {
    flex: 1;
    padding: 12px 16px;
    border: 1px solid var(--border-color);
    border-radius: 20px;
    font-size: 16px;
    outline: none;
}

.chat-input input:focus {
    border-color: var(--primary-color);
}

.chat-input button {
    width: 44px;
    height: 44px;
    background-color: var(--primary-color);
    color: white;
    border: none;
    border-radius: 50%;
    font-size: 18px;
    cursor: pointer;
}

/* Menu Overlay */
.menu-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-color: rgba(0, 0, 0, 0.5);
    display: flex;
    justify-content: flex-end;
    z-index: 100;
}

.menu {
    background-color: white;
    width: 250px;
    padding: 20px;
    box-shadow: -2px 0 10px rgba(0, 0, 0, 0.1);
}

.menu button {
    width: 100%;
    padding: 15px;
    background-color: #ff3b30;
    color: white;
    border: none;
    border-radius: 8px;
    font-size: 16px;
    cursor: pointer;
}

/* Typing indicator */
.typing-indicator {
    display: flex;
    gap: 4px;
    padding: 12px 16px;
}

.typing-indicator span {
    width: 8px;
    height: 8px;
    background-color: #999;
    border-radius: 50%;
    animation: typing 1.4s infinite ease-in-out;
}

.typing-indicator span:nth-child(2) {
    animation-delay: 0.2s;
}

.typing-indicator span:nth-child(3) {
    animation-delay: 0.4s;
}

@keyframes typing {
    0%, 60%, 100% {
        transform: translateY(0);
    }
    30% {
        transform: translateY(-4px);
    }
}
```

**Step 2: Commit**

```bash
mkdir -p web/static
git add web/static/style.css
git commit -m "feat: add mobile-first CSS styles"
```

---

### Task 7.3: Create Chat JavaScript

**Files:**
- Create: `web/static/chat.js`

**Step 1: Create chat client logic**

```javascript
// web/static/chat.js
class ChatClient {
    constructor() {
        this.ws = null;
        this.messagesEl = document.getElementById('messages');
        this.form = document.getElementById('chat-form');
        this.input = document.getElementById('message-input');
        this.menuBtn = document.getElementById('menu-btn');
        this.menuOverlay = document.getElementById('menu-overlay');
        this.logoutBtn = document.getElementById('logout-btn');

        this.init();
    }

    init() {
        // Check auth
        const token = localStorage.getItem('access_token');
        if (!token) {
            window.location.href = '/';
            return;
        }

        this.connect(token);
        this.setupEventListeners();
    }

    connect(token) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat?token=${token}`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log('WebSocket connected');
        };

        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            this.removeTypingIndicator();
            this.addMessage(data.content, 'assistant');
        };

        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            // Try to refresh token and reconnect
            this.refreshAndReconnect();
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
    }

    async refreshAndReconnect() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
            this.logout();
            return;
        }

        try {
            const resp = await fetch('/api/refresh', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({refresh_token: refreshToken})
            });

            if (!resp.ok) {
                throw new Error('Refresh failed');
            }

            const data = await resp.json();
            localStorage.setItem('access_token', data.access_token);

            // Reconnect with new token
            setTimeout(() => this.connect(data.access_token), 1000);
        } catch (err) {
            console.error('Token refresh failed:', err);
            this.logout();
        }
    }

    setupEventListeners() {
        this.form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.sendMessage();
        });

        this.menuBtn.addEventListener('click', () => {
            this.menuOverlay.classList.remove('hidden');
        });

        this.menuOverlay.addEventListener('click', (e) => {
            if (e.target === this.menuOverlay) {
                this.menuOverlay.classList.add('hidden');
            }
        });

        this.logoutBtn.addEventListener('click', () => {
            this.logout();
        });
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
            return;
        }

        // Add user message to UI
        this.addMessage(content, 'user');

        // Show typing indicator
        this.showTypingIndicator();

        // Send to server
        this.ws.send(JSON.stringify({content: content}));

        // Clear input
        this.input.value = '';
    }

    addMessage(content, role) {
        const msgEl = document.createElement('div');
        msgEl.className = `message ${role}`;
        msgEl.textContent = content;
        this.messagesEl.appendChild(msgEl);
        this.scrollToBottom();
    }

    showTypingIndicator() {
        const indicator = document.createElement('div');
        indicator.className = 'message assistant typing-indicator';
        indicator.id = 'typing-indicator';
        indicator.innerHTML = '<span></span><span></span><span></span>';
        this.messagesEl.appendChild(indicator);
        this.scrollToBottom();
    }

    removeTypingIndicator() {
        const indicator = document.getElementById('typing-indicator');
        if (indicator) {
            indicator.remove();
        }
    }

    scrollToBottom() {
        this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
    }

    async logout() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (refreshToken) {
            try {
                await fetch('/api/logout', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({refresh_token: refreshToken})
                });
            } catch (err) {
                console.error('Logout error:', err);
            }
        }

        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        window.location.href = '/';
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    new ChatClient();
});
```

**Step 2: Commit**

```bash
git add web/static/chat.js
git commit -m "feat: add chat client JavaScript with WebSocket support"
```

---

### Task 7.4: Add Template Serving Routes

**Files:**
- Create: `server/pages.go`
- Modify: `server/server.go`

**Step 1: Create template handler**

```go
// server/pages.go
package server

import (
	"html/template"
	"net/http"
	"path/filepath"
)

type PageData struct {
	Title string
}

func (s *Server) loadTemplates() error {
	layout := filepath.Join(s.cfg.WebDir, "templates", "layout.html")

	loginTmpl, err := template.ParseFiles(layout, filepath.Join(s.cfg.WebDir, "templates", "login.html"))
	if err != nil {
		return err
	}
	s.templates["login"] = loginTmpl

	chatTmpl, err := template.ParseFiles(layout, filepath.Join(s.cfg.WebDir, "templates", "chat.html"))
	if err != nil {
		return err
	}
	s.templates["chat"] = chatTmpl

	return nil
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.templates["login"].Execute(w, PageData{Title: "Login"})
}

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	s.templates["chat"].Execute(w, PageData{Title: "Chat"})
}
```

Update server.go:

```go
// Update server/server.go
type Server struct {
	cfg       *config.Config
	db        *db.CoreDB
	jwt       *auth.JWTService
	engine    *assistant.Engine
	router    *http.ServeMux
	templates map[string]*template.Template
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService, engine *assistant.Engine) *Server {
	s := &Server{
		cfg:       cfg,
		db:        coreDB,
		jwt:       jwt,
		engine:    engine,
		router:    http.NewServeMux(),
		templates: make(map[string]*template.Template),
	}

	if cfg.WebDir != "" {
		s.loadTemplates()
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/login", s.handleLogin)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
	s.router.HandleFunc("POST /api/logout", s.handleLogout)
	s.router.HandleFunc("GET /ws/chat", s.handleChat)

	// Page routes
	s.router.HandleFunc("GET /", s.handleLoginPage)
	s.router.HandleFunc("GET /chat", s.handleChatPage)

	// Static files
	if s.cfg.WebDir != "" {
		staticDir := filepath.Join(s.cfg.WebDir, "static")
		s.router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	}
}
```

Add WebDir to config:

```go
// Add to config/config.go Config struct
type Config struct {
	Server   ServerConfig
	LLM      LLMConfig
	JWT      JWTConfig
	DataDir  string
	WebDir   string  // Add this
	InitUser string
	InitPass string
}

// Update Load() function
WebDir: getEnvOrDefault("BOBOT_WEB_DIR", "./web"),
```

**Step 2: Commit**

```bash
git add server/ config/
git commit -m "feat: add template serving and static file routes"
```

---

## Phase 8: Integration and Main Entry Point

### Task 8.1: Create Main Entry Point

**Files:**
- Create: `main.go`

**Step 1: Create main.go**

```go
// main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/server"
	"github.com/esnunes/bobot/tools"
	"github.com/esnunes/bobot/tools/task"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize core database
	coreDB, err := db.NewCoreDB(filepath.Join(cfg.DataDir, "core.db"))
	if err != nil {
		log.Fatalf("Failed to initialize core database: %v", err)
	}
	defer coreDB.Close()

	// Initialize task database
	taskDB, err := task.NewTaskDB(filepath.Join(cfg.DataDir, "tool_task.db"))
	if err != nil {
		log.Fatalf("Failed to initialize task database: %v", err)
	}
	defer taskDB.Close()

	// Create initial user if configured and no users exist
	if cfg.InitUser != "" && cfg.InitPass != "" {
		count, _ := coreDB.UserCount()
		if count == 0 {
			hash, err := auth.HashPassword(cfg.InitPass)
			if err != nil {
				log.Fatalf("Failed to hash initial password: %v", err)
			}
			_, err = coreDB.CreateUser(cfg.InitUser, hash)
			if err != nil {
				log.Fatalf("Failed to create initial user: %v", err)
			}
			log.Printf("Created initial user: %s", cfg.InitUser)
		}
	}

	// Initialize JWT service
	jwtSvc := auth.NewJWTService(cfg.JWT.Secret)

	// Initialize tool registry
	registry := tools.NewRegistry()
	registry.Register(task.NewTaskTool(taskDB))

	// Load skills
	skills, err := assistant.LoadSkills(filepath.Join(cfg.WebDir, "..", "skills"))
	if err != nil {
		log.Printf("Warning: Failed to load skills: %v", err)
	}

	// Initialize LLM provider
	llmProvider := llm.NewAnthropicClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)

	// Initialize assistant engine
	engine := assistant.NewEngine(llmProvider, registry, skills)

	// Initialize HTTP server
	srv := server.NewWithAssistant(cfg, coreDB, jwtSvc, engine)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
```

**Step 2: Commit**

```bash
git add main.go
git commit -m "feat: add main entry point with full initialization"
```

---

### Task 8.2: Create Groceries Skill

**Files:**
- Create: `skills/groceries.md`

**Step 1: Create the groceries skill**

```markdown
---
name: groceries
description: Manage grocery shopping lists
---
When the user wants to manage their grocery list, use the `task` tool with project name "groceries".

Examples:
- "Add milk" → task(command="create", project="groceries", title="milk")
- "What do I need to buy?" → task(command="list", project="groceries", status="pending")
- "Got the eggs" → task(command="update", project="groceries", title="eggs", status="done")
- "Remove bread from the list" → task(command="delete", project="groceries", title="bread")
- "Show me everything on my list" → task(command="list", project="groceries")

Keep responses brief:
- For adding: "Added milk." or "Added milk, eggs, and bread."
- For marking done: "Got it!" or "Crossed off milk."
- For listing: Show the items naturally, e.g., "You need: milk, eggs, bread"
- For removing: "Removed bread."
```

**Step 2: Commit**

```bash
mkdir -p skills
git add skills/
git commit -m "feat: add groceries skill"
```

---

### Task 8.3: Add .gitignore

**Files:**
- Create: `.gitignore`

**Step 1: Create .gitignore**

```
# Data directory
data/

# Build output
bobot-web

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Test coverage
coverage.out

# Environment
.env
.env.local
```

**Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: add .gitignore"
```

---

### Task 8.4: Run Full Test Suite

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 2: Fix any failing tests**

If any tests fail, fix them before proceeding.

**Step 3: Commit any fixes**

```bash
git add -A
git commit -m "fix: resolve test failures"
```

---

### Task 8.5: Build and Manual Test

**Step 1: Build the application**

Run: `go build -o bobot-web .`
Expected: Binary created successfully

**Step 2: Create test environment file**

```bash
export BOBOT_LLM_BASE_URL=https://api.z.ai
export BOBOT_LLM_API_KEY=your-api-key
export BOBOT_LLM_MODEL=glm-4.7
export BOBOT_JWT_SECRET=your-32-character-secret-key-here
export BOBOT_INIT_USER=admin
export BOBOT_INIT_PASS=your-password
```

**Step 3: Run the application**

Run: `./bobot-web`
Expected: Server starts on port 8080

**Step 4: Manual test checklist**

1. Open http://localhost:8080 in mobile browser or responsive mode
2. Login with initial credentials
3. Send "Add milk to my grocery list"
4. Send "What's on my list?"
5. Send "Got the milk"
6. Verify messages persist after page refresh
7. Test logout

**Step 5: Commit**

```bash
git add -A
git commit -m "feat: complete v1 implementation"
```

---

## Summary

This plan implements bobot-web v1 with:

- **Phase 1**: Project foundation (config, core database)
- **Phase 2**: JWT authentication (login, refresh, logout, middleware)
- **Phase 3**: LLM provider (Anthropic-compatible client)
- **Phase 4**: Tool system (registry, task tool with database)
- **Phase 5**: Assistant engine (skills, prompts, tool execution)
- **Phase 6**: WebSocket chat handler
- **Phase 7**: Web UI (templates, CSS, JavaScript)
- **Phase 8**: Integration and main entry point

Total: ~35 bite-sized tasks following TDD methodology with frequent commits.
