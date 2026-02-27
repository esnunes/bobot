// tools/spotify/db.go
package spotify

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type SpotifyDB struct {
	db *sql.DB
}

type OAuthState struct {
	State    string
	UserID   int64
	TopicID  int64
	Verifier string
}

type TokenRecord struct {
	UserID       int64
	AccessToken  string
	RefreshToken string
	TokenType    string
	Expiry       time.Time
}

type TopicLink struct {
	TopicID   int64
	UserID    int64
	CreatedAt time.Time
}

func NewSpotifyDB(dbPath string) (*SpotifyDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}

	sdb := &SpotifyDB{db: db}
	if err := sdb.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return sdb, nil
}

func (s *SpotifyDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS oauth_states (
		state TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		topic_id INTEGER NOT NULL,
		verifier TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tokens (
		user_id INTEGER PRIMARY KEY,
		access_token TEXT NOT NULL,
		refresh_token TEXT NOT NULL,
		token_type TEXT NOT NULL DEFAULT 'Bearer',
		expiry DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS topic_links (
		topic_id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES tokens(user_id),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SpotifyDB) Close() error {
	return s.db.Close()
}

// OAuth state management

func (s *SpotifyDB) SaveOAuthState(state OAuthState) error {
	// Clean up expired states (older than 10 minutes)
	s.db.Exec("DELETE FROM oauth_states WHERE created_at < datetime('now', '-10 minutes')")

	_, err := s.db.Exec(
		"INSERT INTO oauth_states (state, user_id, topic_id, verifier) VALUES (?, ?, ?, ?)",
		state.State, state.UserID, state.TopicID, state.Verifier,
	)
	return err
}

func (s *SpotifyDB) GetAndDeleteOAuthState(state string) (*OAuthState, error) {
	var st OAuthState
	err := s.db.QueryRow(
		"SELECT state, user_id, topic_id, verifier FROM oauth_states WHERE state = ? AND created_at >= datetime('now', '-10 minutes')",
		state,
	).Scan(&st.State, &st.UserID, &st.TopicID, &st.Verifier)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.db.Exec("DELETE FROM oauth_states WHERE state = ?", state)
	return &st, nil
}

// Token management

func (s *SpotifyDB) SaveToken(token TokenRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO tokens (user_id, access_token, refresh_token, token_type, expiry, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_type = excluded.token_type,
			expiry = excluded.expiry,
			updated_at = CURRENT_TIMESTAMP
	`, token.UserID, token.AccessToken, token.RefreshToken, token.TokenType, token.Expiry)
	return err
}

func (s *SpotifyDB) GetToken(userID int64) (*TokenRecord, error) {
	var t TokenRecord
	err := s.db.QueryRow(
		"SELECT user_id, access_token, refresh_token, token_type, expiry FROM tokens WHERE user_id = ?",
		userID,
	).Scan(&t.UserID, &t.AccessToken, &t.RefreshToken, &t.TokenType, &t.Expiry)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SpotifyDB) DeleteToken(userID int64) error {
	_, err := s.db.Exec("DELETE FROM tokens WHERE user_id = ?", userID)
	return err
}

func (s *SpotifyDB) HasToken(userID int64) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tokens WHERE user_id = ?", userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Topic link management

func (s *SpotifyDB) LinkTopic(topicID, userID int64) error {
	_, err := s.db.Exec(
		"INSERT INTO topic_links (topic_id, user_id) VALUES (?, ?) ON CONFLICT(topic_id) DO UPDATE SET user_id = excluded.user_id",
		topicID, userID,
	)
	return err
}

func (s *SpotifyDB) UnlinkTopic(topicID int64) error {
	_, err := s.db.Exec("DELETE FROM topic_links WHERE topic_id = ?", topicID)
	return err
}

func (s *SpotifyDB) GetTopicLink(topicID int64) (*TopicLink, error) {
	var link TopicLink
	err := s.db.QueryRow(
		"SELECT topic_id, user_id, created_at FROM topic_links WHERE topic_id = ?",
		topicID,
	).Scan(&link.TopicID, &link.UserID, &link.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (s *SpotifyDB) GetLinkedTopics(userID int64) ([]int64, error) {
	rows, err := s.db.Query("SELECT topic_id FROM topic_links WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []int64
	for rows.Next() {
		var topicID int64
		if err := rows.Scan(&topicID); err != nil {
			return nil, err
		}
		topics = append(topics, topicID)
	}
	return topics, rows.Err()
}

// Disconnect removes the user's token and all their topic links in a single transaction.
func (s *SpotifyDB) Disconnect(userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM topic_links WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("deleting topic links: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM tokens WHERE user_id = ?", userID); err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}

	return tx.Commit()
}
