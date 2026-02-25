package db

import (
	"database/sql"
	"time"
)

type QuickActionRow struct {
	ID        int64
	Label     string
	Message   string
	Mode      string
	UserID    int64
	TopicID   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c *CoreDB) CreateQuickAction(userID, topicID int64, label, message, mode string) (*QuickActionRow, error) {
	result, err := c.db.Exec(
		"INSERT INTO quick_actions (user_id, topic_id, label, message, mode) VALUES (?, ?, ?, ?, ?)",
		userID, topicID, label, message, mode,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &QuickActionRow{
		ID:        id,
		Label:     label,
		Message:   message,
		Mode:      mode,
		UserID:    userID,
		TopicID:   topicID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (c *CoreDB) GetQuickActionByID(id int64) (*QuickActionRow, error) {
	var qa QuickActionRow
	err := c.db.QueryRow(
		"SELECT id, label, message, mode, user_id, topic_id, created_at, updated_at FROM quick_actions WHERE id = ?",
		id,
	).Scan(&qa.ID, &qa.Label, &qa.Message, &qa.Mode, &qa.UserID, &qa.TopicID, &qa.CreatedAt, &qa.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &qa, nil
}

func (c *CoreDB) GetTopicQuickActions(topicID int64) ([]QuickActionRow, error) {
	rows, err := c.db.Query(
		"SELECT id, label, message, mode, user_id, topic_id, created_at, updated_at FROM quick_actions WHERE topic_id = ? ORDER BY created_at ASC",
		topicID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanQuickActions(rows)
}

func (c *CoreDB) GetTopicQuickActionByLabel(topicID int64, label string) (*QuickActionRow, error) {
	var qa QuickActionRow
	err := c.db.QueryRow(
		"SELECT id, label, message, mode, user_id, topic_id, created_at, updated_at FROM quick_actions WHERE topic_id = ? AND LOWER(label) = LOWER(?)",
		topicID, label,
	).Scan(&qa.ID, &qa.Label, &qa.Message, &qa.Mode, &qa.UserID, &qa.TopicID, &qa.CreatedAt, &qa.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &qa, nil
}

func (c *CoreDB) UpdateQuickAction(id int64, label, message, mode string) error {
	_, err := c.db.Exec(
		"UPDATE quick_actions SET label = ?, message = ?, mode = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		label, message, mode, id,
	)
	return err
}

func (c *CoreDB) DeleteQuickAction(id int64) error {
	_, err := c.db.Exec("DELETE FROM quick_actions WHERE id = ?", id)
	return err
}

func scanQuickActions(rows *sql.Rows) ([]QuickActionRow, error) {
	var actions []QuickActionRow
	for rows.Next() {
		var qa QuickActionRow
		if err := rows.Scan(&qa.ID, &qa.Label, &qa.Message, &qa.Mode, &qa.UserID, &qa.TopicID, &qa.CreatedAt, &qa.UpdatedAt); err != nil {
			return nil, err
		}
		actions = append(actions, qa)
	}
	return actions, rows.Err()
}
