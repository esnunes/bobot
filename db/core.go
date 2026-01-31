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
	DisplayName  string
	Role         string // "admin" or "user"
	Blocked      bool
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
	GroupID       *int64 // nil for 1:1 chats, set for group messages
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

type Group struct {
	ID        int64
	Name      string
	OwnerID   int64
	DeletedAt *time.Time
	CreatedAt time.Time
}

type GroupMember struct {
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

	// Create groups table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS groups (
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

	// Create group_members table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS group_members (
			group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (group_id, user_id)
		)
	`)
	if err != nil {
		return err
	}

	// Migrate: add group_id column to messages
	if err := c.addColumnIfMissing("messages", "group_id", "INTEGER REFERENCES groups(id)"); err != nil {
		return err
	}

	// Create indexes after columns exist
	_, err = c.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_user_context
		ON messages(user_id, id) WHERE context_tokens = 0;
	`)
	if err != nil {
		return err
	}

	// Create index for group messages
	_, err = c.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_group
		ON messages(group_id, id) WHERE group_id IS NOT NULL
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
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND group_id IS NULL
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
		var groupID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupID.Valid {
			m.GroupID = &groupID.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) GetRecentMessages(userID int64, limit int) ([]Message, error) {
	// Get the most recent N messages, but return in chronological order
	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at FROM (
			SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
			FROM messages
			WHERE user_id = ? AND group_id IS NULL
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
		var groupID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupID.Valid {
			m.GroupID = &groupID.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) CreateMessageWithContextThreshold(userID int64, role, content string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest message's context state (for 1:1 chats only)
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE user_id = ? AND group_id IS NULL ORDER BY id DESC LIMIT 1
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
				WHERE user_id = ? AND group_id IS NULL AND context_tokens < ?
				ORDER BY id DESC LIMIT 1
			`, userID, targetThreshold).Scan(&newChunkStartID, &subtractValue)

			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}

			if err == nil {
				// Slide the window
				_, err = c.db.Exec(`
					UPDATE messages SET context_tokens = context_tokens - ?
					WHERE user_id = ? AND group_id IS NULL AND id >= ?
				`, subtractValue, userID, newChunkStartID)
				if err != nil {
					return nil, err
				}

				// Recalculate contextTokens based on updated values
				err = c.db.QueryRow(`
					SELECT tokens, context_tokens FROM messages
					WHERE user_id = ? AND group_id IS NULL ORDER BY id DESC LIMIT 1
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

// CreateGroupMessage creates a message in a group chat.
func (c *CoreDB) CreateGroupMessage(groupID, userID int64, role, content string) (*Message, error) {
	tokens := len(content) / 4

	result, err := c.db.Exec(
		"INSERT INTO messages (group_id, user_id, role, content, tokens) VALUES (?, ?, ?, ?, ?)",
		groupID, userID, role, content, tokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:        id,
		UserID:    userID,
		GroupID:   &groupID,
		Role:      role,
		Content:   content,
		Tokens:    tokens,
		CreatedAt: time.Now(),
	}, nil
}

func (c *CoreDB) GetContextMessages(userID int64) ([]Message, error) {
	// Find the most recent chunk start
	var chunkStartID int64
	err := c.db.QueryRow(`
		SELECT id FROM messages
		WHERE user_id = ? AND group_id IS NULL AND context_tokens = 0
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&chunkStartID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	// Fetch all messages from chunk start to present, excluding command/system roles
	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND group_id IS NULL AND id >= ? AND role IN ('user', 'assistant')
		ORDER BY id ASC
	`, userID, chunkStartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var groupID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupID.Valid {
			m.GroupID = &groupID.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) GetMessagesBefore(userID, beforeID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND group_id IS NULL AND id < ?
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
		var groupID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupID.Valid {
			m.GroupID = &groupID.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) GetMessagesSince(userID int64, since time.Time) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND group_id IS NULL AND created_at > ?
		ORDER BY id ASC
	`, userID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var groupID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupID.Valid {
			m.GroupID = &groupID.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
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

// CreateGroup creates a new group with the given name and owner.
func (c *CoreDB) CreateGroup(name string, ownerID int64) (*Group, error) {
	result, err := c.db.Exec(
		"INSERT INTO groups (name, owner_id) VALUES (?, ?)",
		name, ownerID,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Group{
		ID:        id,
		Name:      name,
		OwnerID:   ownerID,
		CreatedAt: time.Now(),
	}, nil
}

// AddGroupMember adds a user to a group.
func (c *CoreDB) AddGroupMember(groupID, userID int64) error {
	_, err := c.db.Exec(
		"INSERT INTO group_members (group_id, user_id) VALUES (?, ?)",
		groupID, userID,
	)
	return err
}

// RemoveGroupMember removes a user from a group.
func (c *CoreDB) RemoveGroupMember(groupID, userID int64) error {
	_, err := c.db.Exec(
		"DELETE FROM group_members WHERE group_id = ? AND user_id = ?",
		groupID, userID,
	)
	return err
}

// IsGroupMember checks if a user is a member of a group.
func (c *CoreDB) IsGroupMember(groupID, userID int64) (bool, error) {
	var count int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM group_members WHERE group_id = ? AND user_id = ?",
		groupID, userID,
	).Scan(&count)
	return count > 0, err
}

// GetGroupByID retrieves a group by its ID.
func (c *CoreDB) GetGroupByID(id int64) (*Group, error) {
	var group Group
	var deletedAt sql.NullTime
	err := c.db.QueryRow(
		"SELECT id, name, owner_id, deleted_at, created_at FROM groups WHERE id = ? AND deleted_at IS NULL",
		id,
	).Scan(&group.ID, &group.Name, &group.OwnerID, &deletedAt, &group.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		group.DeletedAt = &deletedAt.Time
	}
	return &group, nil
}

// GetUserGroups retrieves all groups a user is a member of.
func (c *CoreDB) GetUserGroups(userID int64) ([]Group, error) {
	rows, err := c.db.Query(`
		SELECT g.id, g.name, g.owner_id, g.deleted_at, g.created_at
		FROM groups g
		JOIN group_members gm ON g.id = gm.group_id
		WHERE gm.user_id = ? AND g.deleted_at IS NULL
		ORDER BY g.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		var deletedAt sql.NullTime
		if err := rows.Scan(&g.ID, &g.Name, &g.OwnerID, &deletedAt, &g.CreatedAt); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			g.DeletedAt = &deletedAt.Time
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// SoftDeleteGroup marks a group as deleted.
func (c *CoreDB) SoftDeleteGroup(groupID int64) error {
	_, err := c.db.Exec(
		"UPDATE groups SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?",
		groupID,
	)
	return err
}

// GetGroupMembers retrieves all members of a group.
func (c *CoreDB) GetGroupMembers(groupID int64) ([]GroupMember, error) {
	rows, err := c.db.Query(`
		SELECT u.id, u.username, u.display_name, gm.joined_at
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id
		WHERE gm.group_id = ?
		ORDER BY gm.joined_at ASC
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []GroupMember
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// GetGroupRecentMessages returns the most recent messages for a group.
func (c *CoreDB) GetGroupRecentMessages(groupID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at FROM (
			SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
			FROM messages
			WHERE group_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) ORDER BY created_at ASC
	`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var groupIDVal sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupIDVal, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupIDVal.Valid {
			m.GroupID = &groupIDVal.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// GetGroupMessagesBefore returns messages before a given ID for a group.
func (c *CoreDB) GetGroupMessagesBefore(groupID, beforeID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE group_id = ? AND id < ?
		ORDER BY id DESC
		LIMIT ?
	`, groupID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var groupIDVal sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupIDVal, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupIDVal.Valid {
			m.GroupID = &groupIDVal.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// GetGroupMessagesSince returns messages since a given time for a group.
func (c *CoreDB) GetGroupMessagesSince(groupID int64, since time.Time) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE group_id = ? AND created_at > ?
		ORDER BY id ASC
	`, groupID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var groupIDVal sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupIDVal, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupIDVal.Valid {
			m.GroupID = &groupIDVal.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// CreateGroupMessageWithContext creates a group message with context tracking.
func (c *CoreDB) CreateGroupMessageWithContext(groupID, userID int64, role, content string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest group message's context state
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE group_id = ? ORDER BY id DESC LIMIT 1
	`, groupID).Scan(&prevTokens, &prevContextTokens)

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
				WHERE group_id = ? AND context_tokens < ?
				ORDER BY id DESC LIMIT 1
			`, groupID, targetThreshold).Scan(&newChunkStartID, &subtractValue)

			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}

			if err == nil {
				_, err = c.db.Exec(`
					UPDATE messages SET context_tokens = context_tokens - ?
					WHERE group_id = ? AND id >= ?
				`, subtractValue, groupID, newChunkStartID)
				if err != nil {
					return nil, err
				}

				err = c.db.QueryRow(`
					SELECT tokens, context_tokens FROM messages
					WHERE group_id = ? ORDER BY id DESC LIMIT 1
				`, groupID).Scan(&prevTokens, &prevContextTokens)
				if err != nil {
					return nil, err
				}
				contextTokens = prevContextTokens + prevTokens + tokens
			}
		}
	}

	result, err := c.db.Exec(
		"INSERT INTO messages (group_id, user_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?, ?)",
		groupID, userID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		UserID:        userID,
		GroupID:       &groupID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
	}, nil
}

// GetGroupContextMessages returns messages in the current context window for a group.
func (c *CoreDB) GetGroupContextMessages(groupID int64) ([]Message, error) {
	var chunkStartID int64
	err := c.db.QueryRow(`
		SELECT id FROM messages
		WHERE group_id = ? AND context_tokens = 0
		ORDER BY id DESC LIMIT 1
	`, groupID).Scan(&chunkStartID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := c.db.Query(`
		SELECT id, user_id, group_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE group_id = ? AND id >= ? AND role IN ('user', 'assistant')
		ORDER BY id ASC
	`, groupID, chunkStartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var groupIDVal sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &groupIDVal, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if groupIDVal.Valid {
			m.GroupID = &groupIDVal.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
