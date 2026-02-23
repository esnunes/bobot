package db

import (
	"database/sql"
	"time"
)

type SkillRow struct {
	ID          int64
	Name        string
	Description string
	Content     string
	UserID      int64
	TopicID     *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (c *CoreDB) CreateSkill(userID int64, topicID *int64, name, description, content string) (*SkillRow, error) {
	var result sql.Result
	var err error
	if topicID != nil {
		result, err = c.db.Exec(
			"INSERT INTO skills (user_id, topic_id, name, description, content) VALUES (?, ?, ?, ?, ?)",
			userID, *topicID, name, description, content,
		)
	} else {
		result, err = c.db.Exec(
			"INSERT INTO skills (user_id, name, description, content) VALUES (?, ?, ?, ?)",
			userID, name, description, content,
		)
	}
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &SkillRow{
		ID:          id,
		Name:        name,
		Description: description,
		Content:     content,
		UserID:      userID,
		TopicID:     topicID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

func (c *CoreDB) GetSkillByID(id int64) (*SkillRow, error) {
	var s SkillRow
	var topicID sql.NullInt64
	err := c.db.QueryRow(
		"SELECT id, name, description, content, user_id, topic_id, created_at, updated_at FROM skills WHERE id = ?",
		id,
	).Scan(&s.ID, &s.Name, &s.Description, &s.Content, &s.UserID, &topicID, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if topicID.Valid {
		s.TopicID = &topicID.Int64
	}
	return &s, nil
}

func (c *CoreDB) GetTopicSkills(topicID int64) ([]SkillRow, error) {
	rows, err := c.db.Query(
		"SELECT id, name, description, content, user_id, topic_id, created_at, updated_at FROM skills WHERE topic_id = ? ORDER BY name",
		topicID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return c.scanSkills(rows)
}

func (c *CoreDB) GetTopicSkillByName(topicID int64, name string) (*SkillRow, error) {
	var s SkillRow
	var tid sql.NullInt64
	err := c.db.QueryRow(
		"SELECT id, name, description, content, user_id, topic_id, created_at, updated_at FROM skills WHERE topic_id = ? AND LOWER(name) = LOWER(?)",
		topicID, name,
	).Scan(&s.ID, &s.Name, &s.Description, &s.Content, &s.UserID, &tid, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tid.Valid {
		s.TopicID = &tid.Int64
	}
	return &s, nil
}

func (c *CoreDB) UpdateSkill(id int64, description, content string) error {
	_, err := c.db.Exec(
		"UPDATE skills SET description = ?, content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		description, content, id,
	)
	return err
}

func (c *CoreDB) DeleteSkill(id int64) error {
	_, err := c.db.Exec("DELETE FROM skills WHERE id = ?", id)
	return err
}

func (c *CoreDB) scanSkills(rows *sql.Rows) ([]SkillRow, error) {
	var skills []SkillRow
	for rows.Next() {
		var s SkillRow
		var topicID sql.NullInt64
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Content, &s.UserID, &topicID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if topicID.Valid {
			s.TopicID = &topicID.Int64
		}
		skills = append(skills, s)
	}
	return skills, rows.Err()
}
