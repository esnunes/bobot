// tools/calendar/db.go
package calendar

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type CalendarDB struct {
	db *sql.DB
}

type TokenRecord struct {
	TopicID      int64
	UserID       int64
	AccessToken  string
	RefreshToken string
	TokenExpiry  time.Time
}

type TopicCalendar struct {
	TopicID      int64
	CalendarID   string
	CalendarName string
	Timezone     string
}

type OAuthState struct {
	State    string
	UserID   int64
	TopicID  int64
	Verifier string
}

func NewCalendarDB(dbPath string) (*CalendarDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}

	cdb := &CalendarDB{db: db}
	if err := cdb.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return cdb, nil
}

func (c *CalendarDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS oauth_states (
		state TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		topic_id INTEGER NOT NULL,
		verifier TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tokens (
		topic_id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		access_token TEXT NOT NULL,
		refresh_token TEXT NOT NULL,
		token_expiry DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS topic_calendars (
		topic_id INTEGER PRIMARY KEY,
		calendar_id TEXT NOT NULL,
		calendar_name TEXT NOT NULL,
		timezone TEXT NOT NULL DEFAULT 'UTC',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := c.db.Exec(schema)
	return err
}

func (c *CalendarDB) Close() error {
	return c.db.Close()
}

// OAuth state management

func (c *CalendarDB) SaveOAuthState(state OAuthState) error {
	// Clean up expired states (older than 10 minutes)
	c.db.Exec("DELETE FROM oauth_states WHERE created_at < datetime('now', '-10 minutes')")

	_, err := c.db.Exec(
		"INSERT INTO oauth_states (state, user_id, topic_id, verifier) VALUES (?, ?, ?, ?)",
		state.State, state.UserID, state.TopicID, state.Verifier,
	)
	return err
}

func (c *CalendarDB) GetAndDeleteOAuthState(state string) (*OAuthState, error) {
	var s OAuthState
	err := c.db.QueryRow(
		"SELECT state, user_id, topic_id, verifier FROM oauth_states WHERE state = ? AND created_at >= datetime('now', '-10 minutes')",
		state,
	).Scan(&s.State, &s.UserID, &s.TopicID, &s.Verifier)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Delete the used state
	c.db.Exec("DELETE FROM oauth_states WHERE state = ?", state)
	return &s, nil
}

// Token management

func (c *CalendarDB) SaveToken(token TokenRecord) error {
	_, err := c.db.Exec(`
		INSERT INTO tokens (topic_id, user_id, access_token, refresh_token, token_expiry, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(topic_id) DO UPDATE SET
			user_id = CASE WHEN excluded.user_id = 0 THEN tokens.user_id ELSE excluded.user_id END,
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_expiry = excluded.token_expiry,
			updated_at = CURRENT_TIMESTAMP
	`, token.TopicID, token.UserID, token.AccessToken, token.RefreshToken, token.TokenExpiry)
	return err
}

func (c *CalendarDB) GetToken(topicID int64) (*TokenRecord, error) {
	var t TokenRecord
	err := c.db.QueryRow(
		"SELECT topic_id, user_id, access_token, refresh_token, token_expiry FROM tokens WHERE topic_id = ?",
		topicID,
	).Scan(&t.TopicID, &t.UserID, &t.AccessToken, &t.RefreshToken, &t.TokenExpiry)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *CalendarDB) DeleteToken(topicID int64) error {
	_, err := c.db.Exec("DELETE FROM tokens WHERE topic_id = ?", topicID)
	return err
}

// Calendar association management

func (c *CalendarDB) SaveTopicCalendar(cal TopicCalendar) error {
	_, err := c.db.Exec(`
		INSERT INTO topic_calendars (topic_id, calendar_id, calendar_name, timezone)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(topic_id) DO UPDATE SET
			calendar_id = excluded.calendar_id,
			calendar_name = excluded.calendar_name,
			timezone = excluded.timezone
	`, cal.TopicID, cal.CalendarID, cal.CalendarName, cal.Timezone)
	return err
}

func (c *CalendarDB) GetTopicCalendar(topicID int64) (*TopicCalendar, error) {
	var cal TopicCalendar
	err := c.db.QueryRow(
		"SELECT topic_id, calendar_id, calendar_name, timezone FROM topic_calendars WHERE topic_id = ?",
		topicID,
	).Scan(&cal.TopicID, &cal.CalendarID, &cal.CalendarName, &cal.Timezone)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cal, nil
}

func (c *CalendarDB) DeleteTopicCalendar(topicID int64) error {
	_, err := c.db.Exec("DELETE FROM topic_calendars WHERE topic_id = ?", topicID)
	return err
}

// Disconnect removes both token and calendar association for a topic.
func (c *CalendarDB) Disconnect(topicID int64) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM tokens WHERE topic_id = ?", topicID); err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM topic_calendars WHERE topic_id = ?", topicID); err != nil {
		return fmt.Errorf("deleting calendar: %w", err)
	}

	return tx.Commit()
}
