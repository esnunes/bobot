// db/core.go
package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
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
	ID            int64
	UserID        int64
	Role          string
	Content       string
	Tokens        int
	ContextTokens int
	CreatedAt     time.Time
}

type CoreDB struct {
	db *sql.DB
}

func NewCoreDB(dbPath string) (*CoreDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
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
		tokens INTEGER NOT NULL DEFAULT 0,
		context_tokens INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_messages_user_context
	ON messages(user_id, id) WHERE context_tokens = 0;
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

func (c *CoreDB) CreateMessageWithContext(userID int64, role, content string) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest message's context state
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE user_id = ? ORDER BY id DESC LIMIT 1
	`, userID).Scan(&prevTokens, &prevContextTokens)

	var contextTokens int
	if err == sql.ErrNoRows {
		// First message - starts a new chunk
		contextTokens = 0
	} else if err != nil {
		return nil, err
	} else {
		// Continue the chunk
		contextTokens = prevContextTokens + prevTokens + tokens
	}

	result, err := c.db.Exec(
		"INSERT INTO messages (user_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?)",
		userID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		UserID:        userID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
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

func (c *CoreDB) CreateMessageWithContextThreshold(userID int64, role, content string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest message's context state
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE user_id = ? ORDER BY id DESC LIMIT 1
	`, userID).Scan(&prevTokens, &prevContextTokens)

	var contextTokens int
	if err == sql.ErrNoRows {
		// First message - starts a new chunk
		contextTokens = 0
	} else if err != nil {
		return nil, err
	} else {
		// Calculate what context_tokens would be
		contextTokens = prevContextTokens + prevTokens + tokens

		// Check if we need to reset the chunk
		if contextTokens > tokensMax {
			targetThreshold := tokensMax - tokensStart

			// Find the new chunk start
			var newChunkStartID int64
			var subtractValue int
			err := c.db.QueryRow(`
				SELECT id, context_tokens FROM messages
				WHERE user_id = ? AND context_tokens < ?
				ORDER BY id DESC LIMIT 1
			`, userID, targetThreshold).Scan(&newChunkStartID, &subtractValue)

			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}

			if err == nil {
				// Slide the window
				_, err = c.db.Exec(`
					UPDATE messages SET context_tokens = context_tokens - ?
					WHERE user_id = ? AND id >= ?
				`, subtractValue, userID, newChunkStartID)
				if err != nil {
					return nil, err
				}

				// Recalculate contextTokens based on updated values
				err = c.db.QueryRow(`
					SELECT tokens, context_tokens FROM messages
					WHERE user_id = ? ORDER BY id DESC LIMIT 1
				`, userID).Scan(&prevTokens, &prevContextTokens)
				if err != nil {
					return nil, err
				}
				contextTokens = prevContextTokens + prevTokens + tokens
			}
		}
	}

	result, err := c.db.Exec(
		"INSERT INTO messages (user_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?)",
		userID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		UserID:        userID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
	}, nil
}

func (c *CoreDB) GetContextMessages(userID int64) ([]Message, error) {
	// Find the most recent chunk start
	var chunkStartID int64
	err := c.db.QueryRow(`
		SELECT id FROM messages
		WHERE user_id = ? AND context_tokens = 0
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&chunkStartID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	// Fetch all messages from chunk start to present
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND id >= ?
		ORDER BY id ASC
	`, userID, chunkStartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) GetMessagesBefore(userID, beforeID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND id < ?
		ORDER BY id DESC
		LIMIT ?
	`, userID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) GetMessagesSince(userID int64, since time.Time) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND created_at > ?
		ORDER BY id ASC
	`, userID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
