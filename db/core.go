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

// BobotUserID is the reserved user ID for the Bobot assistant.
// This user is created during migration with ID 0.
const BobotUserID = int64(0)

const WelcomeMessage = `
👋 Olá! Seja muito bem-vindo(a)!

Quero te conhecer melhor para poder te ajudar da melhor forma possível 😊.

Se puder, me conte rapidinho:

- Qual o seu nome?
- Onde você mora (cidade e país)?

🌎 **Idioma / Language**

Se você preferir outro idioma, pode responder nele.

If you prefer another language, just reply in that language. I will continue assisting you using it.
`

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	DisplayName  string
	Role         string // "admin" or "user"
	Blocked      bool
	CreatedAt    time.Time
}

type Message struct {
	ID            int64
	SenderID      int64  // who sent the message (user ID or BobotUserID)
	ReceiverID    *int64 // who receives (NULL for topic messages)
	TopicID       *int64 // nil for 1:1 chats, set for topic messages
	Role          string
	Content       string
	Tokens        int
	ContextTokens int
	CreatedAt     time.Time
}

type Invite struct {
	ID        int64
	Code      string
	CreatedBy int64
	UsedBy    *int64
	UsedAt    *time.Time
	Revoked   bool
	CreatedAt time.Time
}

type Topic struct {
	ID        int64
	Name      string
	OwnerID   int64
	DeletedAt *time.Time
	CreatedAt time.Time
}

type TopicMember struct {
	UserID      int64
	Username    string
	DisplayName string
	JoinedAt    time.Time
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
	// Create tables first (without indexes that depend on new columns)
	tables := `
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
	`
	_, err := c.db.Exec(tables)
	if err != nil {
		return err
	}

	// Migrate existing databases: add tokens column if missing
	if err := c.addColumnIfMissing("messages", "tokens", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Migrate existing databases: add context_tokens column if missing
	if err := c.addColumnIfMissing("messages", "context_tokens", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

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

	// Migrate: rename groups table to topics (if groups exists and topics doesn't)
	if err := c.renameTableIfExists("groups", "topics"); err != nil {
		return err
	}

	// Migrate: rename group_members table to topic_members
	if err := c.renameTableIfExists("group_members", "topic_members"); err != nil {
		return err
	}

	// Migrate: rename group_id column to topic_id in topic_members table
	if err := c.renameColumnIfExists("topic_members", "group_id", "topic_id"); err != nil {
		return err
	}

	// Create topics table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS topics (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			owner_id INTEGER NOT NULL REFERENCES users(id),
			deleted_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Create topic_members table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS topic_members (
			topic_id INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (topic_id, user_id)
		)
	`)
	if err != nil {
		return err
	}

	// Migrate: rename group_id column to topic_id in messages
	if err := c.renameColumnIfExists("messages", "group_id", "topic_id"); err != nil {
		return err
	}

	// Migrate: add topic_id column to messages (for fresh databases)
	if err := c.addColumnIfMissing("messages", "topic_id", "INTEGER REFERENCES topics(id)"); err != nil {
		return err
	}

	// Migrate: Create bobot user with ID 0 for assistant messages
	_, err = c.db.Exec(`
		INSERT OR IGNORE INTO users (id, username, password_hash, display_name, role, blocked)
		VALUES (0, '_bobot', '!', 'Bobot', 'system', 0)
	`)
	if err != nil {
		return err
	}

	// Migrate: add receiver_id column to messages
	if err := c.addColumnIfMissing("messages", "receiver_id", "INTEGER REFERENCES users(id)"); err != nil {
		return err
	}

	// Migrate: rename user_id to sender_id
	if err := c.renameColumnIfExists("messages", "user_id", "sender_id"); err != nil {
		return err
	}

	// Migrate existing private messages: infer sender/receiver from role
	// User messages: sender=original user, receiver=bobot (0)
	// Assistant/system messages: sender=bobot (0), receiver=original user
	_, err = c.db.Exec(`
		UPDATE messages
		SET sender_id = CASE WHEN role IN ('assistant', 'system') THEN 0 ELSE sender_id END,
		    receiver_id = CASE WHEN role IN ('assistant', 'system') THEN sender_id ELSE 0 END
		WHERE topic_id IS NULL AND receiver_id IS NULL
	`)
	if err != nil {
		return err
	}

	// Drop old index if exists
	_, _ = c.db.Exec(`DROP INDEX IF EXISTS idx_messages_user_context`)

	// Create new indexes for private chat (bidirectional)
	_, err = c.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_private_chat
		ON messages(sender_id, receiver_id, id) WHERE topic_id IS NULL
	`)
	if err != nil {
		return err
	}

	_, err = c.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_context
		ON messages(sender_id, receiver_id, id) WHERE context_tokens = 0 AND topic_id IS NULL
	`)
	if err != nil {
		return err
	}

	// Drop old index and create new index for topic messages
	_, _ = c.db.Exec(`DROP INDEX IF EXISTS idx_messages_group`)

	_, err = c.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_topic
		ON messages(topic_id, id) WHERE topic_id IS NOT NULL
	`)
	if err != nil {
		return err
	}

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

	// Migrate: add case-insensitive unique index for active topic names
	_, err = c.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_topics_name_active ON topics(LOWER(name)) WHERE deleted_at IS NULL
	`)
	if err != nil {
		return err
	}

	// Create user_profiles table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_profiles (
			user_id INTEGER PRIMARY KEY REFERENCES users(id),
			content TEXT NOT NULL DEFAULT '',
			last_message_id INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	return nil
}

func (c *CoreDB) addColumnIfMissing(table, column, definition string) error {
	var count int
	err := c.db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info(?)
		WHERE name = ?
	`, table, column).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		_, err = c.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CoreDB) renameColumnIfExists(table, oldName, newName string) error {
	// Check if old column exists
	var oldCount int
	err := c.db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info(?)
		WHERE name = ?
	`, table, oldName).Scan(&oldCount)
	if err != nil {
		return err
	}

	// Check if new column already exists
	var newCount int
	err = c.db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info(?)
		WHERE name = ?
	`, table, newName).Scan(&newCount)
	if err != nil {
		return err
	}

	// Only rename if old exists and new doesn't
	if oldCount > 0 && newCount == 0 {
		_, err = c.db.Exec("ALTER TABLE " + table + " RENAME COLUMN " + oldName + " TO " + newName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CoreDB) renameTableIfExists(oldName, newName string) error {
	// Check if old table exists
	var oldCount int
	err := c.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name=?
	`, oldName).Scan(&oldCount)
	if err != nil {
		return err
	}

	// Check if new table already exists
	var newCount int
	err = c.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name=?
	`, newName).Scan(&newCount)
	if err != nil {
		return err
	}

	// Only rename if old exists and new doesn't
	if oldCount > 0 && newCount == 0 {
		_, err = c.db.Exec("ALTER TABLE " + oldName + " RENAME TO " + newName)
		if err != nil {
			return err
		}
	}

	return nil
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
		Role:         "user",
		CreatedAt:    time.Now(),
	}, nil
}

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

func (c *CoreDB) UserCount() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (c *CoreDB) CreateMessage(senderID, receiverID int64, role, content string) (*Message, error) {
	result, err := c.db.Exec(
		"INSERT INTO messages (sender_id, receiver_id, role, content) VALUES (?, ?, ?, ?)",
		senderID, receiverID, role, content,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:         id,
		SenderID:   senderID,
		ReceiverID: &receiverID,
		Role:       role,
		Content:    content,
		CreatedAt:  time.Now(),
	}, nil
}

func (c *CoreDB) CreateMessageWithContext(senderID, receiverID int64, role, content string) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest message's context state for this private chat (bidirectional)
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE topic_id IS NULL
		  AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
		ORDER BY id DESC LIMIT 1
	`, senderID, receiverID, receiverID, senderID).Scan(&prevTokens, &prevContextTokens)

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
		"INSERT INTO messages (sender_id, receiver_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?, ?)",
		senderID, receiverID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		SenderID:      senderID,
		ReceiverID:    &receiverID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
	}, nil
}

// GetPrivateChatMessages returns messages for a private chat between a user and Bobot.
func (c *CoreDB) GetPrivateChatMessages(userID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id IS NULL
		  AND ((sender_id = ? AND receiver_id = 0) OR (sender_id = 0 AND receiver_id = ?))
		ORDER BY id ASC
		LIMIT ?
	`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

func (c *CoreDB) scanMessages(rows *sql.Rows) ([]Message, error) {
	var messages []Message
	for rows.Next() {
		var m Message
		var receiverID, topicID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.SenderID, &receiverID, &topicID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if receiverID.Valid {
			m.ReceiverID = &receiverID.Int64
		}
		if topicID.Valid {
			m.TopicID = &topicID.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// GetPrivateChatRecentMessages returns the most recent messages for a private chat.
func (c *CoreDB) GetPrivateChatRecentMessages(userID int64, limit int) ([]Message, error) {
	// Get the most recent N messages, but return in chronological order
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at FROM (
			SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
			FROM messages
			WHERE topic_id IS NULL
			  AND ((sender_id = ? AND receiver_id = 0) OR (sender_id = 0 AND receiver_id = ?))
			ORDER BY id DESC
			LIMIT ?
		) ORDER BY id ASC
	`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// CreatePrivateMessageWithContextThreshold creates a private message with context window management.
func (c *CoreDB) CreatePrivateMessageWithContextThreshold(senderID, receiverID int64, role, content string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest message's context state (for private chats, bidirectional)
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE topic_id IS NULL
		  AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
		ORDER BY id DESC LIMIT 1
	`, senderID, receiverID, receiverID, senderID).Scan(&prevTokens, &prevContextTokens)

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
				WHERE topic_id IS NULL
				  AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
				  AND context_tokens < ?
				ORDER BY id DESC LIMIT 1
			`, senderID, receiverID, receiverID, senderID, targetThreshold).Scan(&newChunkStartID, &subtractValue)

			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}

			if err == nil {
				// Slide the window - update all messages in this private chat
				_, err = c.db.Exec(`
					UPDATE messages SET context_tokens = context_tokens - ?
					WHERE topic_id IS NULL
					  AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
					  AND id >= ?
				`, subtractValue, senderID, receiverID, receiverID, senderID, newChunkStartID)
				if err != nil {
					return nil, err
				}

				// Recalculate contextTokens based on updated values
				err = c.db.QueryRow(`
					SELECT tokens, context_tokens FROM messages
					WHERE topic_id IS NULL
					  AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
					ORDER BY id DESC LIMIT 1
				`, senderID, receiverID, receiverID, senderID).Scan(&prevTokens, &prevContextTokens)
				if err != nil {
					return nil, err
				}
				contextTokens = prevContextTokens + prevTokens + tokens
			}
		}
	}

	result, err := c.db.Exec(
		"INSERT INTO messages (sender_id, receiver_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?, ?)",
		senderID, receiverID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		SenderID:      senderID,
		ReceiverID:    &receiverID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
	}, nil
}

// CreateTopicMessage creates a message in a topic chat.
func (c *CoreDB) CreateTopicMessage(topicID, senderID int64, role, content string) (*Message, error) {
	tokens := len(content) / 4

	result, err := c.db.Exec(
		"INSERT INTO messages (topic_id, sender_id, role, content, tokens) VALUES (?, ?, ?, ?, ?)",
		topicID, senderID, role, content, tokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:        id,
		SenderID:  senderID,
		TopicID:   &topicID,
		Role:      role,
		Content:   content,
		Tokens:    tokens,
		CreatedAt: time.Now(),
	}, nil
}

// GetPrivateChatContextMessages returns messages in the current context window for a private chat.
func (c *CoreDB) GetPrivateChatContextMessages(userID int64) ([]Message, error) {
	// Find the most recent chunk start for this private chat
	var chunkStartID int64
	err := c.db.QueryRow(`
		SELECT id FROM messages
		WHERE topic_id IS NULL
		  AND ((sender_id = ? AND receiver_id = 0) OR (sender_id = 0 AND receiver_id = ?))
		  AND context_tokens = 0
		ORDER BY id DESC LIMIT 1
	`, userID, userID).Scan(&chunkStartID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	// Fetch all messages from chunk start to present, excluding command/system roles
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id IS NULL
		  AND ((sender_id = ? AND receiver_id = 0) OR (sender_id = 0 AND receiver_id = ?))
		  AND id >= ?
		  AND role IN ('user', 'assistant')
		ORDER BY id ASC
	`, userID, userID, chunkStartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// GetPrivateChatMessagesBefore returns messages before a given ID for a private chat.
func (c *CoreDB) GetPrivateChatMessagesBefore(userID, beforeID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id IS NULL
		  AND ((sender_id = ? AND receiver_id = 0) OR (sender_id = 0 AND receiver_id = ?))
		  AND id < ?
		ORDER BY id DESC
		LIMIT ?
	`, userID, userID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// GetPrivateChatMessagesSince returns messages since a given time for a private chat.
func (c *CoreDB) GetPrivateChatMessagesSince(userID int64, since time.Time) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id IS NULL
		  AND ((sender_id = ? AND receiver_id = 0) OR (sender_id = 0 AND receiver_id = ?))
		  AND created_at > ?
		ORDER BY id ASC
	`, userID, userID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

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

// CreateTopic creates a new topic with the given name and owner.
func (c *CoreDB) CreateTopic(name string, ownerID int64) (*Topic, error) {
	result, err := c.db.Exec(
		"INSERT INTO topics (name, owner_id) VALUES (?, ?)",
		name, ownerID,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Topic{
		ID:        id,
		Name:      name,
		OwnerID:   ownerID,
		CreatedAt: time.Now(),
	}, nil
}

// AddTopicMember adds a user to a topic.
func (c *CoreDB) AddTopicMember(topicID, userID int64) error {
	_, err := c.db.Exec(
		"INSERT INTO topic_members (topic_id, user_id) VALUES (?, ?)",
		topicID, userID,
	)
	return err
}

// RemoveTopicMember removes a user from a topic.
func (c *CoreDB) RemoveTopicMember(topicID, userID int64) error {
	_, err := c.db.Exec(
		"DELETE FROM topic_members WHERE topic_id = ? AND user_id = ?",
		topicID, userID,
	)
	return err
}

// IsTopicMember checks if a user is a member of a topic.
func (c *CoreDB) IsTopicMember(topicID, userID int64) (bool, error) {
	var count int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM topic_members WHERE topic_id = ? AND user_id = ?",
		topicID, userID,
	).Scan(&count)
	return count > 0, err
}

// GetTopicByID retrieves a topic by its ID.
func (c *CoreDB) GetTopicByID(id int64) (*Topic, error) {
	var topic Topic
	var deletedAt sql.NullTime
	err := c.db.QueryRow(
		"SELECT id, name, owner_id, deleted_at, created_at FROM topics WHERE id = ? AND deleted_at IS NULL",
		id,
	).Scan(&topic.ID, &topic.Name, &topic.OwnerID, &deletedAt, &topic.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		topic.DeletedAt = &deletedAt.Time
	}
	return &topic, nil
}

// GetTopicByName retrieves an active topic by name (case-insensitive).
func (c *CoreDB) GetTopicByName(name string) (*Topic, error) {
	var topic Topic
	var deletedAt sql.NullTime
	err := c.db.QueryRow(
		"SELECT id, name, owner_id, deleted_at, created_at FROM topics WHERE LOWER(name) = LOWER(?) AND deleted_at IS NULL",
		name,
	).Scan(&topic.ID, &topic.Name, &topic.OwnerID, &deletedAt, &topic.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		topic.DeletedAt = &deletedAt.Time
	}
	return &topic, nil
}

// GetUserTopics retrieves all topics a user is a member of.
func (c *CoreDB) GetUserTopics(userID int64) ([]Topic, error) {
	rows, err := c.db.Query(`
		SELECT t.id, t.name, t.owner_id, t.deleted_at, t.created_at
		FROM topics t
		JOIN topic_members tm ON t.id = tm.topic_id
		WHERE tm.user_id = ? AND t.deleted_at IS NULL
		ORDER BY t.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var t Topic
		var deletedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.OwnerID, &deletedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			t.DeletedAt = &deletedAt.Time
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// SoftDeleteTopic marks a topic as deleted.
func (c *CoreDB) SoftDeleteTopic(topicID int64) error {
	_, err := c.db.Exec(
		"UPDATE topics SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?",
		topicID,
	)
	return err
}

// GetTopicMembers retrieves all members of a topic.
func (c *CoreDB) GetTopicMembers(topicID int64) ([]TopicMember, error) {
	rows, err := c.db.Query(`
		SELECT u.id, u.username, u.display_name, tm.joined_at
		FROM topic_members tm
		JOIN users u ON tm.user_id = u.id
		WHERE tm.topic_id = ?
		ORDER BY tm.joined_at ASC
	`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []TopicMember
	for rows.Next() {
		var m TopicMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// GetTopicRecentMessages returns the most recent messages for a topic.
func (c *CoreDB) GetTopicRecentMessages(topicID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at FROM (
			SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
			FROM messages
			WHERE topic_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) ORDER BY created_at ASC
	`, topicID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// GetTopicMessagesBefore returns messages before a given ID for a topic.
func (c *CoreDB) GetTopicMessagesBefore(topicID, beforeID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id = ? AND id < ?
		ORDER BY id DESC
		LIMIT ?
	`, topicID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// GetTopicMessagesSince returns messages since a given time for a topic.
func (c *CoreDB) GetTopicMessagesSince(topicID int64, since time.Time) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id = ? AND created_at > ?
		ORDER BY id ASC
	`, topicID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// CreateTopicMessageWithContext creates a topic message with context tracking.
func (c *CoreDB) CreateTopicMessageWithContext(topicID, senderID int64, role, content string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest topic message's context state
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE topic_id = ? ORDER BY id DESC LIMIT 1
	`, topicID).Scan(&prevTokens, &prevContextTokens)

	var contextTokens int
	if err == sql.ErrNoRows {
		contextTokens = 0
	} else if err != nil {
		return nil, err
	} else {
		contextTokens = prevContextTokens + prevTokens + tokens

		if contextTokens > tokensMax {
			targetThreshold := tokensMax - tokensStart

			var newChunkStartID int64
			var subtractValue int
			err := c.db.QueryRow(`
				SELECT id, context_tokens FROM messages
				WHERE topic_id = ? AND context_tokens < ?
				ORDER BY id DESC LIMIT 1
			`, topicID, targetThreshold).Scan(&newChunkStartID, &subtractValue)

			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}

			if err == nil {
				_, err = c.db.Exec(`
					UPDATE messages SET context_tokens = context_tokens - ?
					WHERE topic_id = ? AND id >= ?
				`, subtractValue, topicID, newChunkStartID)
				if err != nil {
					return nil, err
				}

				err = c.db.QueryRow(`
					SELECT tokens, context_tokens FROM messages
					WHERE topic_id = ? ORDER BY id DESC LIMIT 1
				`, topicID).Scan(&prevTokens, &prevContextTokens)
				if err != nil {
					return nil, err
				}
				contextTokens = prevContextTokens + prevTokens + tokens
			}
		}
	}

	result, err := c.db.Exec(
		"INSERT INTO messages (topic_id, sender_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?, ?)",
		topicID, senderID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		SenderID:      senderID,
		TopicID:       &topicID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
	}, nil
}

// GetTopicContextMessages returns messages in the current context window for a topic.
func (c *CoreDB) GetTopicContextMessages(topicID int64) ([]Message, error) {
	var chunkStartID int64
	err := c.db.QueryRow(`
		SELECT id FROM messages
		WHERE topic_id = ? AND context_tokens = 0
		ORDER BY id DESC LIMIT 1
	`, topicID).Scan(&chunkStartID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id = ? AND id >= ? AND role IN ('user', 'assistant')
		ORDER BY id ASC
	`, topicID, chunkStartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

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
		userID, tokenIssuedAt.UTC(),
	).Scan(&count)
	return count > 0, err
}

func (c *CoreDB) DeleteOldSessionRevocations(olderThan time.Time) (int64, error) {
	result, err := c.db.Exec(
		"DELETE FROM session_revocations WHERE revoked_at < ?",
		olderThan.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
