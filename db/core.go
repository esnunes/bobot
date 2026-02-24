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
	SenderID      int64 // who sent the message (user ID or BobotUserID)
	TopicID       int64
	Role          string
	Content       string
	RawContent    string
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
	ID          int64
	Name        string
	OwnerID     int64
	AutoRespond bool
	DeletedAt   *time.Time
	CreatedAt   time.Time
}

type TopicMember struct {
	UserID      int64
	Username    string
	DisplayName string
	JoinedAt    time.Time
	Muted       bool
	AutoRead    bool
}

type PushSubscription struct {
	ID        int64
	UserID    int64
	Endpoint  string
	P256DH    string
	Auth      string
	CreatedAt time.Time
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

	// Migrate: rename user_id to sender_id
	if err := c.renameColumnIfExists("messages", "user_id", "sender_id"); err != nil {
		return err
	}

	// Migrate: add raw_content column to messages
	if err := c.addColumnIfMissing("messages", "raw_content", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Legacy migration: receiver_id was used for private chat before unification.
	// If the column still exists, run the legacy migration steps then drop it.
	hasReceiverID, err := c.columnExists("messages", "receiver_id")
	if err != nil {
		return err
	}
	if hasReceiverID {
		// Migrate existing private messages: infer sender/receiver from role
		_, err = c.db.Exec(`
			UPDATE messages
			SET sender_id = CASE WHEN role IN ('assistant', 'system') THEN 0 ELSE sender_id END,
			    receiver_id = CASE WHEN role IN ('assistant', 'system') THEN sender_id ELSE 0 END
			WHERE topic_id IS NULL AND receiver_id IS NULL
		`)
		if err != nil {
			return err
		}
	}

	// Drop old index if exists
	_, _ = c.db.Exec(`DROP INDEX IF EXISTS idx_messages_user_context`)

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

	// Note: idx_topics_name_active was previously created here but is now dropped below
	// (topic names are no longer globally unique — each user has a "bobot" topic).
	// Kept as no-op for migration ordering.

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

	// Create skills table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			user_id INTEGER NOT NULL REFERENCES users(id),
			topic_id INTEGER REFERENCES topics(id) ON DELETE CASCADE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// Unique skill name per private chat scope (case-insensitive)
	_, err = c.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_private_name
		ON skills(user_id, LOWER(name)) WHERE topic_id IS NULL
	`)
	if err != nil {
		return err
	}

	// Unique skill name per topic scope (case-insensitive)
	_, err = c.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_topic_name
		ON skills(topic_id, LOWER(name)) WHERE topic_id IS NOT NULL
	`)
	if err != nil {
		return err
	}

	// Create push_subscriptions table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS push_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			endpoint TEXT NOT NULL UNIQUE,
			p256dh TEXT NOT NULL,
			auth TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	_, err = c.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user_id
		ON push_subscriptions(user_id)
	`)
	if err != nil {
		return err
	}

	// Create chat_read_status table for unread message tracking
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_read_status (
			user_id INTEGER NOT NULL REFERENCES users(id),
			topic_id INTEGER REFERENCES topics(id),
			last_read_message_id INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return err
	}

	// Unique index for topic chat read status
	_, err = c.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_read_status_topic
		ON chat_read_status(user_id, topic_id) WHERE topic_id IS NOT NULL
	`)
	if err != nil {
		return err
	}

	// Migrate: add muted column to topic_members
	if err := c.addColumnIfMissing("topic_members", "muted", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Migrate: add auto_read column to topic_members
	if err := c.addColumnIfMissing("topic_members", "auto_read", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Migrate: add auto_respond column to topics
	if err := c.addColumnIfMissing("topics", "auto_respond", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Migrate: drop unique topic name index (topic names are now scoped to members, not globally unique)
	_, err = c.db.Exec(`DROP INDEX IF EXISTS idx_topics_name_active`)
	if err != nil {
		return err
	}

	// Migrate: create bobot topics for existing users and move private messages.
	// This migration references receiver_id, so only run if the column exists.
	if hasReceiverID {
		if err := c.migratePrivateChatsToTopics(); err != nil {
			return err
		}
	}

	// Cleanup: drop old private chat indexes and receiver_id column
	_, _ = c.db.Exec(`DROP INDEX IF EXISTS idx_messages_private_chat`)
	_, _ = c.db.Exec(`DROP INDEX IF EXISTS idx_messages_context`)
	_, _ = c.db.Exec(`DROP INDEX IF EXISTS idx_chat_read_status_private`)

	if err := c.dropColumnIfExists("messages", "receiver_id"); err != nil {
		return err
	}

	return nil
}

// migratePrivateChatsToTopics creates "bobot" topics for existing users who don't have one,
// and moves their private messages (topic_id IS NULL) into the bobot topic.
// Also migrates chat_read_status rows for private chat.
// This migration is idempotent.
func (c *CoreDB) migratePrivateChatsToTopics() error {
	// Find users who have private messages but no bobot topic
	rows, err := c.db.Query(`
		SELECT DISTINCT u.id FROM users u
		WHERE u.id != ?
		AND NOT EXISTS (
			SELECT 1 FROM topics t WHERE t.name = 'bobot' AND t.owner_id = u.id AND t.deleted_at IS NULL
		)
		AND EXISTS (
			SELECT 1 FROM messages m WHERE m.topic_id IS NULL AND (m.sender_id = u.id OR m.receiver_id = u.id)
		)
	`, BobotUserID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		userIDs = append(userIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, userID := range userIDs {
		topic, err := c.CreateBobotTopic(userID)
		if err != nil {
			return err
		}

		// Move private messages into the bobot topic
		_, err = c.db.Exec(`
			UPDATE messages SET topic_id = ?
			WHERE topic_id IS NULL
			AND (sender_id = ? OR receiver_id = ?)
		`, topic.ID, userID, userID)
		if err != nil {
			return err
		}

		// Migrate chat_read_status for private chat
		_, err = c.db.Exec(`
			UPDATE chat_read_status SET topic_id = ?
			WHERE topic_id IS NULL AND user_id = ?
		`, topic.ID, userID)
		if err != nil {
			return err
		}
	}

	// Migrate orphaned private skills (topic_id IS NULL) into each user's bobot topic.
	// This covers users who already had a bobot topic from a previous migration run.
	_, err = c.db.Exec(`
		UPDATE skills SET topic_id = (
			SELECT t.id FROM topics t
			WHERE t.name = 'bobot' AND t.owner_id = skills.user_id AND t.deleted_at IS NULL
			LIMIT 1
		)
		WHERE topic_id IS NULL
		AND EXISTS (
			SELECT 1 FROM topics t
			WHERE t.name = 'bobot' AND t.owner_id = skills.user_id AND t.deleted_at IS NULL
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

func (c *CoreDB) columnExists(table, column string) (bool, error) {
	var count int
	err := c.db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info(?)
		WHERE name = ?
	`, table, column).Scan(&count)
	return count > 0, err
}

func (c *CoreDB) dropColumnIfExists(table, column string) error {
	exists, err := c.columnExists(table, column)
	if err != nil {
		return err
	}
	if exists {
		_, err = c.db.Exec("ALTER TABLE " + table + " DROP COLUMN " + column)
	}
	return err
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

func (c *CoreDB) scanMessages(rows *sql.Rows) ([]Message, error) {
	var messages []Message
	for rows.Next() {
		var m Message
		var topicID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.SenderID, &topicID, &m.Role, &m.Content, &m.RawContent, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.TopicID = topicID.Int64
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// CreateTopicMessage creates a message in a topic chat.
func (c *CoreDB) CreateTopicMessage(topicID, senderID int64, role, content, rawContent string) (*Message, error) {
	tokens := len(rawContent) / 4

	result, err := c.db.Exec(
		"INSERT INTO messages (topic_id, sender_id, role, content, raw_content, tokens) VALUES (?, ?, ?, ?, ?, ?)",
		topicID, senderID, role, content, rawContent, tokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:         id,
		SenderID:   senderID,
		TopicID:    topicID,
		Role:       role,
		Content:    content,
		RawContent: rawContent,
		Tokens:     tokens,
		CreatedAt:  time.Now(),
	}, nil
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

// GetUserProfile returns the profile content and last processed message ID for a user.
// Returns empty content and 0 if no profile exists yet.
func (c *CoreDB) GetUserProfile(userID int64) (string, int64, error) {
	var content string
	var lastMessageID int64
	err := c.db.QueryRow(
		"SELECT content, last_message_id FROM user_profiles WHERE user_id = ?",
		userID,
	).Scan(&content, &lastMessageID)

	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	return content, lastMessageID, nil
}

// UpsertUserProfile inserts or replaces a user's profile.
func (c *CoreDB) UpsertUserProfile(userID int64, content string, lastMessageID int64) error {
	_, err := c.db.Exec(
		"INSERT INTO user_profiles (user_id, content, last_message_id, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP) ON CONFLICT(user_id) DO UPDATE SET content = excluded.content, last_message_id = excluded.last_message_id, updated_at = CURRENT_TIMESTAMP",
		userID, content, lastMessageID,
	)
	return err
}

// GetUserMessagesSince returns user-role messages sent by a user since a given message ID.
func (c *CoreDB) GetUserMessagesSince(userID int64, sinceMessageID int64) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, topic_id, role, content, raw_content, tokens, context_tokens, created_at
		FROM messages
		WHERE sender_id = ? AND role = 'user' AND id > ?
		ORDER BY id ASC
	`, userID, sinceMessageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// ListActiveUsers returns all non-blocked, non-system users.
func (c *CoreDB) ListActiveUsers() ([]User, error) {
	rows, err := c.db.Query(`
		SELECT id, username, password_hash, display_name, role, blocked, created_at
		FROM users
		WHERE id != 0 AND blocked = 0
		ORDER BY id ASC
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

// CreateBobotTopic creates a "bobot" topic for a user with auto_respond enabled,
// and adds the user as a member.
func (c *CoreDB) CreateBobotTopic(userID int64) (*Topic, error) {
	topic, err := c.CreateTopic("bobot", userID)
	if err != nil {
		return nil, err
	}

	if err := c.SetTopicAutoRespond(topic.ID, true); err != nil {
		return nil, err
	}
	topic.AutoRespond = true

	if err := c.AddTopicMember(topic.ID, userID); err != nil {
		return nil, err
	}

	return topic, nil
}

// GetUserBobotTopic returns the user's "bobot" topic, or nil if not found.
func (c *CoreDB) GetUserBobotTopic(userID int64) (*Topic, error) {
	var topic Topic
	err := c.db.QueryRow(`
		SELECT t.id, t.name, t.owner_id, t.auto_respond, t.created_at
		FROM topics t
		WHERE t.name = 'bobot' AND t.owner_id = ? AND t.deleted_at IS NULL
	`, userID).Scan(&topic.ID, &topic.Name, &topic.OwnerID, &topic.AutoRespond, &topic.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &topic, nil
}

// AddTopicMember adds a user to a topic.
func (c *CoreDB) AddTopicMember(topicID, userID int64) error {
	_, err := c.db.Exec(
		"INSERT INTO topic_members (topic_id, user_id) VALUES (?, ?)",
		topicID, userID,
	)
	return err
}

// SetTopicAutoRespond enables or disables auto_respond for a topic.
func (c *CoreDB) SetTopicAutoRespond(topicID int64, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	_, err := c.db.Exec("UPDATE topics SET auto_respond = ? WHERE id = ?", val, topicID)
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
		"SELECT id, name, owner_id, auto_respond, deleted_at, created_at FROM topics WHERE id = ? AND deleted_at IS NULL",
		id,
	).Scan(&topic.ID, &topic.Name, &topic.OwnerID, &topic.AutoRespond, &deletedAt, &topic.CreatedAt)

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
func (c *CoreDB) ListAllTopics() ([]Topic, error) {
	rows, err := c.db.Query(`
		SELECT id, name, owner_id, deleted_at, created_at
		FROM topics WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)
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

func (c *CoreDB) GetUserTopics(userID int64) ([]Topic, error) {
	rows, err := c.db.Query(`
		SELECT t.id, t.name, t.owner_id, t.auto_respond, t.deleted_at, t.created_at
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
		if err := rows.Scan(&t.ID, &t.Name, &t.OwnerID, &t.AutoRespond, &deletedAt, &t.CreatedAt); err != nil {
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
		SELECT u.id, u.username, u.display_name, tm.joined_at, tm.muted, tm.auto_read
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
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.JoinedAt, &m.Muted, &m.AutoRead); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// SetTopicMemberMuted sets the muted state for a user in a topic.
func (c *CoreDB) SetTopicMemberMuted(topicID, userID int64, muted bool) error {
	_, err := c.db.Exec(
		"UPDATE topic_members SET muted = ? WHERE topic_id = ? AND user_id = ?",
		muted, topicID, userID,
	)
	return err
}

// SetTopicMemberAutoRead sets the auto-read state for a user in a topic.
func (c *CoreDB) SetTopicMemberAutoRead(topicID, userID int64, autoRead bool) error {
	_, err := c.db.Exec(
		"UPDATE topic_members SET auto_read = ? WHERE topic_id = ? AND user_id = ?",
		autoRead, topicID, userID,
	)
	return err
}

// GetTopicRecentMessages returns the most recent messages for a topic.
func (c *CoreDB) GetTopicRecentMessages(topicID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, topic_id, role, content, raw_content, tokens, context_tokens, created_at FROM (
			SELECT id, sender_id, topic_id, role, content, raw_content, tokens, context_tokens, created_at
			FROM messages
			WHERE topic_id = ?
			  AND content != ''
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
		SELECT id, sender_id, topic_id, role, content, raw_content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id = ? AND id < ?
		  AND content != ''
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
		SELECT id, sender_id, topic_id, role, content, raw_content, tokens, context_tokens, created_at
		FROM messages
		WHERE topic_id = ? AND created_at > ?
		  AND content != ''
		ORDER BY id ASC
	`, topicID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}

// CreateTopicMessageWithContext creates a topic message with context tracking.
func (c *CoreDB) CreateTopicMessageWithContext(topicID, senderID int64, role, content, rawContent string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(rawContent) / 4

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
		"INSERT INTO messages (topic_id, sender_id, role, content, raw_content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?, ?, ?)",
		topicID, senderID, role, content, rawContent, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		SenderID:      senderID,
		TopicID:       topicID,
		Role:          role,
		Content:       content,
		RawContent:    rawContent,
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
		SELECT id, sender_id, topic_id, role, content, raw_content, tokens, context_tokens, created_at
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

// SavePushSubscription stores or replaces a push subscription for a user.
func (c *CoreDB) SavePushSubscription(userID int64, endpoint, p256dh, auth string) error {
	_, err := c.db.Exec(
		"INSERT OR REPLACE INTO push_subscriptions (user_id, endpoint, p256dh, auth) VALUES (?, ?, ?, ?)",
		userID, endpoint, p256dh, auth,
	)
	return err
}

// DeletePushSubscription removes a push subscription by endpoint.
func (c *CoreDB) DeletePushSubscription(endpoint string) error {
	_, err := c.db.Exec("DELETE FROM push_subscriptions WHERE endpoint = ?", endpoint)
	return err
}

// DeletePushSubscriptionsByUser removes all push subscriptions for a user.
func (c *CoreDB) DeletePushSubscriptionsByUser(userID int64) error {
	_, err := c.db.Exec("DELETE FROM push_subscriptions WHERE user_id = ?", userID)
	return err
}

// GetPushSubscriptions returns all push subscriptions for a user.
func (c *CoreDB) GetPushSubscriptions(userID int64) ([]PushSubscription, error) {
	rows, err := c.db.Query(
		"SELECT id, user_id, endpoint, p256dh, auth, created_at FROM push_subscriptions WHERE user_id = ?",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []PushSubscription
	for rows.Next() {
		var s PushSubscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256DH, &s.Auth, &s.CreatedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// MarkChatRead updates the last read message ID for a user in a topic.
func (c *CoreDB) MarkChatRead(userID int64, topicID int64, messageID int64) error {
	_, err := c.db.Exec(`
		INSERT INTO chat_read_status (user_id, topic_id, last_read_message_id)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, topic_id) WHERE topic_id IS NOT NULL
		DO UPDATE SET last_read_message_id = excluded.last_read_message_id
	`, userID, topicID, messageID)
	return err
}

// GetUnreadChats returns a map of topic IDs with unread messages for the user.
// All chats (including the bobot topic) are tracked via topics.
// Missing chat_read_status rows are treated as "all read" (no unread indicator).
func (c *CoreDB) GetUnreadChats(userID int64) (map[int64]bool, error) {
	rows, err := c.db.Query(`
		SELECT t.id,
		       COALESCE(MAX(m.id), 0) > COALESCE(crs.last_read_message_id, 0) AS has_unread
		FROM topics t
		JOIN topic_members tm ON tm.topic_id = t.id AND tm.user_id = ? AND tm.auto_read = 0
		LEFT JOIN messages m ON m.topic_id = t.id
		LEFT JOIN chat_read_status crs ON crs.user_id = ? AND crs.topic_id = t.id
		WHERE t.deleted_at IS NULL
		GROUP BY t.id
	`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	unreads := make(map[int64]bool)
	for rows.Next() {
		var topicID int64
		var hasUnread bool
		if err := rows.Scan(&topicID, &hasUnread); err != nil {
			return nil, err
		}
		if hasUnread {
			unreads[topicID] = true
		}
	}
	return unreads, rows.Err()
}

// GetLatestTopicMessageID returns the ID of the most recent message in a topic.
func (c *CoreDB) GetLatestTopicMessageID(topicID int64) (int64, error) {
	var id int64
	err := c.db.QueryRow(`
		SELECT COALESCE(MAX(id), 0) FROM messages WHERE topic_id = ?
	`, topicID).Scan(&id)
	return id, err
}

// ReadPosition holds a user's read position with display info.
type ReadPosition struct {
	UserID      int64
	DisplayName string
	LastReadID  int64
}

// GetTopicReadPositions returns read positions for all members of a topic,
// excluding Bobot (user_id=0). Only returns users who have a chat_read_status row.
func (c *CoreDB) GetTopicReadPositions(topicID int64) ([]ReadPosition, error) {
	rows, err := c.db.Query(`
		SELECT crs.user_id,
		       CASE WHEN u.display_name != '' THEN u.display_name ELSE u.username END,
		       crs.last_read_message_id
		FROM chat_read_status crs
		JOIN users u ON crs.user_id = u.id
		WHERE crs.topic_id = ? AND crs.user_id != ?
	`, topicID, BobotUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var positions []ReadPosition
	for rows.Next() {
		var p ReadPosition
		if err := rows.Scan(&p.UserID, &p.DisplayName, &p.LastReadID); err != nil {
			return nil, err
		}
		positions = append(positions, p)
	}
	return positions, rows.Err()
}
