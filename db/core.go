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
