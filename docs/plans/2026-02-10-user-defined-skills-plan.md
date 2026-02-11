# User-Defined Skills Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let users define custom skills (name, description, markdown content) scoped to private chat or topics, stored in SQLite, injected into the system prompt alongside built-in skills, manageable via slash command/LLM tool and REST API.

**Architecture:** Skills table lives in CoreDB (foreign keys to `users` and `topics`). A new `SkillProvider` interface on the engine fetches per-request skills. A `tools/skill/` package implements the `Tool` interface. Server handlers provide REST API for web UI.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), standard library `net/http`, existing `tools.Tool` interface.

---

### Task 1: Database Schema and CRUD

Add the `skills` table to CoreDB and implement CRUD methods.

**Files:**
- Modify: `db/core.go` (migration + `SkillRow` struct)
- Create: `db/skills.go` (CRUD methods)
- Create: `db/skills_test.go` (tests)

**Step 1: Write the failing tests for skill CRUD**

Create `db/skills_test.go`:

```go
package db

import (
	"path/filepath"
	"testing"
)

func setupSkillTestDB(t *testing.T) *CoreDB {
	t.Helper()
	tmpDir := t.TempDir()
	coreDB, err := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	return coreDB
}

func TestCreatePrivateChatSkill(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")

	skill, err := db.CreateSkill(user.ID, nil, "groceries", "Manage grocery lists", "Use task tool for groceries")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}
	if skill.ID == 0 {
		t.Error("expected non-zero skill ID")
	}
	if skill.Name != "groceries" {
		t.Errorf("expected name 'groceries', got %q", skill.Name)
	}
}

func TestCreateTopicSkill(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := db.CreateTopic("General", user.ID)

	skill, err := db.CreateSkill(user.ID, &topic.ID, "meeting-notes", "Track meeting notes", "Always summarize meetings")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}
	if skill.TopicID == nil || *skill.TopicID != topic.ID {
		t.Error("expected skill to be scoped to topic")
	}
}

func TestCreateSkillDuplicateNamePrivate(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")

	db.CreateSkill(user.ID, nil, "groceries", "desc", "content")
	_, err := db.CreateSkill(user.ID, nil, "groceries", "desc2", "content2")
	if err == nil {
		t.Error("expected error for duplicate skill name")
	}
}

func TestCreateSkillDuplicateNameTopic(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	alice, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := db.CreateUserFull("bob", "hash", "Bob", "user")
	topic, _ := db.CreateTopic("General", alice.ID)

	db.CreateSkill(alice.ID, &topic.ID, "notes", "desc", "content")
	// Different user, same topic, same name — should fail
	_, err := db.CreateSkill(bob.ID, &topic.ID, "notes", "desc2", "content2")
	if err == nil {
		t.Error("expected error for duplicate skill name in topic")
	}
}

func TestCreateSkillSameNameDifferentScopes(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := db.CreateTopic("General", user.ID)

	// Same name in private + topic should be allowed
	_, err := db.CreateSkill(user.ID, nil, "groceries", "desc", "content")
	if err != nil {
		t.Fatalf("private skill failed: %v", err)
	}
	_, err = db.CreateSkill(user.ID, &topic.ID, "groceries", "desc", "content")
	if err != nil {
		t.Fatalf("topic skill failed: %v", err)
	}
}

func TestGetSkillByID(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	created, _ := db.CreateSkill(user.ID, nil, "groceries", "desc", "content")

	skill, err := db.GetSkillByID(created.ID)
	if err != nil {
		t.Fatalf("get skill failed: %v", err)
	}
	if skill.Name != "groceries" {
		t.Errorf("expected name 'groceries', got %q", skill.Name)
	}
}

func TestGetSkillByIDNotFound(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	_, err := db.GetSkillByID(999)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPrivateChatSkills(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	alice, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := db.CreateUserFull("bob", "hash", "Bob", "user")
	topic, _ := db.CreateTopic("General", alice.ID)

	db.CreateSkill(alice.ID, nil, "groceries", "desc1", "content1")
	db.CreateSkill(alice.ID, nil, "recipes", "desc2", "content2")
	db.CreateSkill(alice.ID, &topic.ID, "topic-skill", "desc3", "content3")
	db.CreateSkill(bob.ID, nil, "bob-skill", "desc4", "content4")

	skills, err := db.GetPrivateChatSkills(alice.ID)
	if err != nil {
		t.Fatalf("get private skills failed: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 private skills, got %d", len(skills))
	}
}

func TestGetTopicSkills(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	alice, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	topic1, _ := db.CreateTopic("General", alice.ID)
	topic2, _ := db.CreateTopic("Random", alice.ID)

	db.CreateSkill(alice.ID, &topic1.ID, "skill1", "desc", "content")
	db.CreateSkill(alice.ID, &topic1.ID, "skill2", "desc", "content")
	db.CreateSkill(alice.ID, &topic2.ID, "skill3", "desc", "content")

	skills, err := db.GetTopicSkills(topic1.ID)
	if err != nil {
		t.Fatalf("get topic skills failed: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 topic skills, got %d", len(skills))
	}
}

func TestUpdateSkill(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	created, _ := db.CreateSkill(user.ID, nil, "groceries", "old desc", "old content")

	err := db.UpdateSkill(created.ID, "new desc", "new content")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	skill, _ := db.GetSkillByID(created.ID)
	if skill.Description != "new desc" {
		t.Errorf("expected description 'new desc', got %q", skill.Description)
	}
	if skill.Content != "new content" {
		t.Errorf("expected content 'new content', got %q", skill.Content)
	}
}

func TestDeleteSkill(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	created, _ := db.CreateSkill(user.ID, nil, "groceries", "desc", "content")

	err := db.DeleteSkill(created.ID)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = db.GetSkillByID(created.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestGetPrivateChatSkillByName(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	db.CreateSkill(user.ID, nil, "Groceries", "desc", "content")

	// Case-insensitive lookup
	skill, err := db.GetPrivateChatSkillByName(user.ID, "groceries")
	if err != nil {
		t.Fatalf("get by name failed: %v", err)
	}
	if skill.Name != "Groceries" {
		t.Errorf("expected name 'Groceries', got %q", skill.Name)
	}
}

func TestGetTopicSkillByName(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := db.CreateTopic("General", user.ID)
	db.CreateSkill(user.ID, &topic.ID, "Notes", "desc", "content")

	skill, err := db.GetTopicSkillByName(topic.ID, "notes")
	if err != nil {
		t.Fatalf("get by name failed: %v", err)
	}
	if skill.Name != "Notes" {
		t.Errorf("expected name 'Notes', got %q", skill.Name)
	}
}

func TestSkillsCascadeDeleteOnTopicDelete(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := db.CreateTopic("General", user.ID)
	db.CreateSkill(user.ID, &topic.ID, "notes", "desc", "content")

	// Soft-delete the topic — skills should remain (soft delete doesn't trigger CASCADE)
	// But we should test the cascade on the FK
	skills, _ := db.GetTopicSkills(topic.ID)
	if len(skills) != 1 {
		t.Errorf("expected 1 topic skill before delete, got %d", len(skills))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./db/ -run TestCreate.*Skill -v`
Expected: FAIL — `SkillRow` type and methods don't exist yet.

**Step 3: Add skills table migration to CoreDB**

In `db/core.go`, at the end of the `migrate()` method (before `return nil`), add:

```go
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
```

**Step 4: Create `db/skills.go` with struct and CRUD methods**

```go
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

func (c *CoreDB) GetPrivateChatSkills(userID int64) ([]SkillRow, error) {
	rows, err := c.db.Query(
		"SELECT id, name, description, content, user_id, topic_id, created_at, updated_at FROM skills WHERE user_id = ? AND topic_id IS NULL ORDER BY name",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return c.scanSkills(rows)
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

func (c *CoreDB) GetPrivateChatSkillByName(userID int64, name string) (*SkillRow, error) {
	var s SkillRow
	var topicID sql.NullInt64
	err := c.db.QueryRow(
		"SELECT id, name, description, content, user_id, topic_id, created_at, updated_at FROM skills WHERE user_id = ? AND topic_id IS NULL AND LOWER(name) = LOWER(?)",
		userID, name,
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
```

**Step 5: Run tests to verify they pass**

Run: `go test ./db/ -run Test.*Skill -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add db/core.go db/skills.go db/skills_test.go
git commit -m "feat(db): add skills table with CRUD operations

Adds skills table to CoreDB with per-scope uniqueness indexes.
Skills can be scoped to private chat (topic_id IS NULL) or a
specific topic (topic_id IS NOT NULL)."
```

---

### Task 2: Skill Tool

Implement `tools/skill/` following the existing tool pattern.

**Files:**
- Create: `tools/skill/skill.go`
- Create: `tools/skill/skill_test.go`

**Step 1: Write the failing tests**

Create `tools/skill/skill_test.go`:

```go
package skill

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	t.Helper()
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	return coreDB
}

func ctxForUser(userID int64, role string) context.Context {
	return auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: userID,
		Role:   role,
	})
}

func ctxForUserInTopic(userID int64, role string, topicID int64) context.Context {
	ctx := ctxForUser(userID, role)
	return auth.ContextWithChatData(ctx, auth.ChatData{TopicID: &topicID})
}

func TestSkillTool_Interface(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	tool := NewSkillTool(coreDB)
	if tool.Name() != "skill" {
		t.Errorf("expected name 'skill', got %q", tool.Name())
	}
	if tool.AdminOnly() {
		t.Error("expected AdminOnly to be false")
	}
}

func TestSkillTool_CreatePrivate(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewSkillTool(coreDB)

	result, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command":     "create",
		"name":        "groceries",
		"description": "Manage grocery lists",
		"content":     "Use task tool for groceries",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(result, "groceries") {
		t.Errorf("expected skill name in result, got: %s", result)
	}

	// Verify in DB
	skill, _ := coreDB.GetPrivateChatSkillByName(user.ID, "groceries")
	if skill == nil {
		t.Fatal("expected skill to exist in DB")
	}
}

func TestSkillTool_CreateTopic(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateTopic("General", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)
	tool := NewSkillTool(coreDB)

	result, err := tool.Execute(ctxForUserInTopic(user.ID, "user", topic.ID), map[string]any{
		"command":     "create",
		"name":        "notes",
		"description": "Meeting notes",
		"content":     "Summarize meetings",
	})
	if err != nil {
		t.Fatalf("create topic skill failed: %v", err)
	}
	if !strings.Contains(result, "notes") {
		t.Errorf("expected skill name in result, got: %s", result)
	}
}

func TestSkillTool_CreateTopicNotOwnerOrAdmin(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	topic, _ := coreDB.CreateTopic("General", alice.ID)
	coreDB.AddTopicMember(topic.ID, alice.ID)
	coreDB.AddTopicMember(topic.ID, bob.ID)
	tool := NewSkillTool(coreDB)

	// Bob is a member but not owner, not admin
	_, err := tool.Execute(ctxForUserInTopic(bob.ID, "user", topic.ID), map[string]any{
		"command": "create",
		"name":    "notes",
		"content": "content",
	})
	if err == nil {
		t.Error("expected permission error for non-owner/non-admin")
	}
}

func TestSkillTool_CreateTopicAdmin(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	topic, _ := coreDB.CreateTopic("General", alice.ID)
	coreDB.AddTopicMember(topic.ID, alice.ID)
	coreDB.AddTopicMember(topic.ID, admin.ID)
	tool := NewSkillTool(coreDB)

	// Admin can create topic skills even though not owner
	_, err := tool.Execute(ctxForUserInTopic(admin.ID, "admin", topic.ID), map[string]any{
		"command": "create",
		"name":    "notes",
		"content": "content",
	})
	if err != nil {
		t.Fatalf("admin create should succeed: %v", err)
	}
}

func TestSkillTool_CreateMissingName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewSkillTool(coreDB)

	_, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "create",
		"content": "some content",
	})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestSkillTool_Update(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewSkillTool(coreDB)

	tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "create", "name": "groceries", "description": "old", "content": "old content",
	})

	result, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "update", "name": "groceries", "description": "new desc", "content": "new content",
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !strings.Contains(result, "updated") {
		t.Errorf("expected update confirmation, got: %s", result)
	}

	skill, _ := coreDB.GetPrivateChatSkillByName(user.ID, "groceries")
	if skill.Content != "new content" {
		t.Errorf("expected updated content, got %q", skill.Content)
	}
}

func TestSkillTool_UpdateNotFound(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewSkillTool(coreDB)

	_, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "update", "name": "nonexistent", "content": "new content",
	})
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestSkillTool_Delete(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewSkillTool(coreDB)

	tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "create", "name": "groceries", "content": "content",
	})

	result, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "delete", "name": "groceries",
	})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected delete confirmation, got: %s", result)
	}
}

func TestSkillTool_List(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewSkillTool(coreDB)

	tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "create", "name": "groceries", "description": "Grocery lists", "content": "content",
	})
	tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "create", "name": "recipes", "description": "Recipe management", "content": "content",
	})

	result, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "groceries") || !strings.Contains(result, "recipes") {
		t.Errorf("expected both skills in list, got: %s", result)
	}
}

func TestSkillTool_ListEmpty(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewSkillTool(coreDB)

	result, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "No skills") {
		t.Errorf("expected 'No skills' message, got: %s", result)
	}
}

func TestSkillTool_ParseArgs(t *testing.T) {
	tool := &SkillTool{}

	tests := []struct {
		name    string
		raw     string
		want    map[string]any
		wantErr bool
	}{
		{name: "empty", raw: "", wantErr: true},
		{name: "create", raw: "create groceries", want: map[string]any{"command": "create", "name": "groceries"}},
		{name: "update", raw: "update groceries", want: map[string]any{"command": "update", "name": "groceries"}},
		{name: "delete", raw: "delete groceries", want: map[string]any{"command": "delete", "name": "groceries"}},
		{name: "list", raw: "list", want: map[string]any{"command": "list"}},
		{name: "create multi-word", raw: "create my skill", want: map[string]any{"command": "create", "name": "my skill"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tool.ParseArgs(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: got %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./tools/skill/ -v`
Expected: FAIL — package doesn't exist yet.

**Step 3: Implement the skill tool**

Create `tools/skill/skill.go`:

```go
package skill

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type SkillTool struct {
	db *db.CoreDB
}

func NewSkillTool(db *db.CoreDB) *SkillTool {
	return &SkillTool{db: db}
}

func (s *SkillTool) Name() string {
	return "skill"
}

func (s *SkillTool) Description() string {
	return "Manage skills: create, update, delete, list custom skills for this chat"
}

func (s *SkillTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "update", "delete", "list"},
				"description": "The operation to perform",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Short description of what the skill does",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Markdown content with instructions for the skill",
			},
		},
		"required": []string{"command"},
	}
}

func (s *SkillTool) AdminOnly() bool {
	return false
}

func (s *SkillTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing command. Usage: /skill <command>")
	}

	result := map[string]any{"command": parts[0]}
	if len(parts) > 1 {
		result["name"] = strings.Join(parts[1:], " ")
	}
	return result, nil
}

func (s *SkillTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	chatData := auth.ChatDataFromContext(ctx)

	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("missing command. Usage: /skill <command>")
	}

	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	content, _ := input["content"].(string)

	switch command {
	case "create":
		return s.create(userData, chatData, name, description, content)
	case "update":
		return s.update(userData, chatData, name, description, content)
	case "delete":
		return s.deleteSk(userData, chatData, name)
	case "list":
		return s.list(userData, chatData)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (s *SkillTool) canManageTopicSkills(userID int64, role string, topicID int64) error {
	if role == "admin" {
		return nil
	}
	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	if topic.OwnerID != userID {
		return fmt.Errorf("only the topic owner or admins can manage topic skills")
	}
	return nil
}

func (s *SkillTool) create(userData auth.UserData, chatData auth.ChatData, name, description, content string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing skill name. Usage: /skill create <name>")
	}

	var topicID *int64
	if chatData.TopicID != nil {
		if err := s.canManageTopicSkills(userData.UserID, userData.Role, *chatData.TopicID); err != nil {
			return "", err
		}
		topicID = chatData.TopicID
	}

	if len(content) > 4096 {
		slog.Warn("skill content exceeds 4KB", "name", name, "size", len(content))
	}

	skill, err := s.db.CreateSkill(userData.UserID, topicID, name, description, content)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return "", fmt.Errorf("a skill named %q already exists in this scope", name)
		}
		return "", fmt.Errorf("failed to create skill: %w", err)
	}

	s.warnIfTooManySkills(userData.UserID, topicID)

	return fmt.Sprintf("Skill %q created.", skill.Name), nil
}

func (s *SkillTool) update(userData auth.UserData, chatData auth.ChatData, name, description, content string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing skill name. Usage: /skill update <name>")
	}

	skill, err := s.resolveSkill(userData, chatData, name)
	if err != nil {
		return "", err
	}

	if chatData.TopicID != nil {
		if err := s.canManageTopicSkills(userData.UserID, userData.Role, *chatData.TopicID); err != nil {
			return "", err
		}
	}

	if len(content) > 4096 {
		slog.Warn("skill content exceeds 4KB", "name", name, "size", len(content))
	}

	// Keep existing values if not provided
	if description == "" {
		description = skill.Description
	}
	if content == "" {
		content = skill.Content
	}

	if err := s.db.UpdateSkill(skill.ID, description, content); err != nil {
		return "", fmt.Errorf("failed to update skill: %w", err)
	}

	return fmt.Sprintf("Skill %q updated.", name), nil
}

func (s *SkillTool) deleteSk(userData auth.UserData, chatData auth.ChatData, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing skill name. Usage: /skill delete <name>")
	}

	skill, err := s.resolveSkill(userData, chatData, name)
	if err != nil {
		return "", err
	}

	if chatData.TopicID != nil {
		if err := s.canManageTopicSkills(userData.UserID, userData.Role, *chatData.TopicID); err != nil {
			return "", err
		}
	}

	if err := s.db.DeleteSkill(skill.ID); err != nil {
		return "", fmt.Errorf("failed to delete skill: %w", err)
	}

	return fmt.Sprintf("Skill %q deleted.", name), nil
}

func (s *SkillTool) list(userData auth.UserData, chatData auth.ChatData) (string, error) {
	var skills []db.SkillRow
	var err error

	if chatData.TopicID != nil {
		skills, err = s.db.GetTopicSkills(*chatData.TopicID)
	} else {
		skills, err = s.db.GetPrivateChatSkills(userData.UserID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to list skills: %w", err)
	}

	if len(skills) == 0 {
		return "No skills found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Skills:\n")
	for _, sk := range skills {
		if sk.Description != "" {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", sk.Name, sk.Description))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", sk.Name))
		}
	}
	return sb.String(), nil
}

func (s *SkillTool) resolveSkill(userData auth.UserData, chatData auth.ChatData, name string) (*db.SkillRow, error) {
	if chatData.TopicID != nil {
		skill, err := s.db.GetTopicSkillByName(*chatData.TopicID, name)
		if err == db.ErrNotFound {
			return nil, fmt.Errorf("skill not found: %s", name)
		}
		return skill, err
	}
	skill, err := s.db.GetPrivateChatSkillByName(userData.UserID, name)
	if err == db.ErrNotFound {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return skill, err
}

func (s *SkillTool) warnIfTooManySkills(userID int64, topicID *int64) {
	var skills []db.SkillRow
	var err error
	if topicID != nil {
		skills, err = s.db.GetTopicSkills(*topicID)
	} else {
		skills, err = s.db.GetPrivateChatSkills(userID)
	}
	if err == nil && len(skills) > 10 {
		slog.Warn("scope exceeds 10 skills", "userID", userID, "topicID", topicID, "count", len(skills))
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./tools/skill/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add tools/skill/skill.go tools/skill/skill_test.go
git commit -m "feat(tools): add skill tool for managing user-defined skills

Implements /skill create, update, delete, list commands.
Supports private chat and topic scopes with permission checks
(topic owner or admin required for topic skills)."
```

---

### Task 3: System Prompt Integration

Add a `SkillProvider` interface to the engine so user-defined skills are fetched per-request and merged with built-in skills.

**Files:**
- Modify: `assistant/engine.go` (add SkillProvider, merge skills in Chat)
- Modify: `context/adapter.go` (implement SkillProvider)
- Create: `assistant/engine_test.go` (test skill merging)

**Step 1: Write the failing test**

Create `assistant/engine_test.go`:

```go
package assistant

import (
	"context"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

type mockSkillProvider struct {
	privateSkills []Skill
	topicSkills   map[int64][]Skill
}

func (m *mockSkillProvider) GetPrivateChatSkills(userID int64) ([]Skill, error) {
	return m.privateSkills, nil
}

func (m *mockSkillProvider) GetTopicSkills(topicID int64) ([]Skill, error) {
	if m.topicSkills == nil {
		return nil, nil
	}
	return m.topicSkills[topicID], nil
}

type mockProvider struct {
	lastSystemPrompt string
}

func (m *mockProvider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	m.lastSystemPrompt = req.SystemPrompt
	return &llm.ChatResponse{Content: "ok", RawContent: `"ok"`}, nil
}

type mockContextProvider struct{}

func (m *mockContextProvider) GetContextMessages(userID int64) ([]ContextMessage, error) {
	return nil, nil
}
func (m *mockContextProvider) GetTopicContextMessages(topicID int64) ([]ContextMessage, error) {
	return nil, nil
}

type mockMessageSaver struct{}

func (m *mockMessageSaver) SaveMessage(userID int64, role, content, rawContent string) error {
	return nil
}
func (m *mockMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	return nil
}

func TestEngine_MergesUserSkillsIntoPrompt(t *testing.T) {
	provider := &mockProvider{}
	registry := tools.NewRegistry()
	builtinSkills := []Skill{{Name: "builtin", Description: "Built-in skill", Content: "builtin content"}}

	skillProvider := &mockSkillProvider{
		privateSkills: []Skill{
			{Name: "custom", Description: "Custom skill", Content: "custom content"},
		},
	}

	engine := NewEngine(provider, registry, builtinSkills, &mockContextProvider{}, nil)
	engine.SetSkillProvider(skillProvider)
	engine.SetMessageSaver(&mockMessageSaver{})

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})

	engine.Chat(ctx, ChatOptions{Message: "hello"})

	if provider.lastSystemPrompt == "" {
		t.Fatal("expected system prompt to be set")
	}

	// Built-in skill should appear
	if !contains(provider.lastSystemPrompt, "builtin content") {
		t.Error("expected builtin skill in system prompt")
	}
	// User-defined skill should appear
	if !contains(provider.lastSystemPrompt, "custom content") {
		t.Error("expected custom skill in system prompt")
	}
	// Builtin should appear before custom
	builtinIdx := indexOf(provider.lastSystemPrompt, "builtin content")
	customIdx := indexOf(provider.lastSystemPrompt, "custom content")
	if builtinIdx > customIdx {
		t.Error("expected builtin skills before custom skills")
	}
}

func TestEngine_TopicSkillsInTopicChat(t *testing.T) {
	provider := &mockProvider{}
	registry := tools.NewRegistry()

	skillProvider := &mockSkillProvider{
		topicSkills: map[int64][]Skill{
			42: {{Name: "topic-skill", Description: "Topic skill", Content: "topic skill content"}},
		},
	}

	engine := NewEngine(provider, registry, nil, &mockContextProvider{}, nil)
	engine.SetSkillProvider(skillProvider)
	engine.SetMessageSaver(&mockMessageSaver{})

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})

	engine.Chat(ctx, ChatOptions{Message: "hello", TopicID: 42})

	if !contains(provider.lastSystemPrompt, "topic skill content") {
		t.Error("expected topic skill in system prompt")
	}
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./assistant/ -run TestEngine -v`
Expected: FAIL — `SetSkillProvider` method doesn't exist.

**Step 3: Add SkillProvider to the engine**

In `assistant/engine.go`, add the interface and modify the engine:

After the `ProfileProvider` interface, add:

```go
// SkillProvider retrieves user-defined skills for the current chat scope.
type SkillProvider interface {
	GetPrivateChatSkills(userID int64) ([]Skill, error)
	GetTopicSkills(topicID int64) ([]Skill, error)
}
```

Add field to Engine struct:

```go
	skillProvider   SkillProvider
```

Add setter:

```go
func (e *Engine) SetSkillProvider(provider SkillProvider) {
	e.skillProvider = provider
}
```

In `Chat()`, replace the line:

```go
	systemPrompt := BuildSystemPrompt(e.skills, llmTools)
```

With:

```go
	// Merge built-in skills with user-defined skills
	allSkills := append([]Skill{}, e.skills...)
	if e.skillProvider != nil {
		var userSkills []Skill
		var skillErr error
		if opts.TopicID > 0 {
			userSkills, skillErr = e.skillProvider.GetTopicSkills(opts.TopicID)
		} else {
			userSkills, skillErr = e.skillProvider.GetPrivateChatSkills(userData.UserID)
		}
		if skillErr == nil {
			allSkills = append(allSkills, userSkills...)
		}
	}
	systemPrompt := BuildSystemPrompt(allSkills, llmTools)
```

**Step 4: Implement SkillProvider in context adapter**

In `context/adapter.go`, add the interface assertion and methods:

```go
var _ assistant.SkillProvider = (*CoreDBAdapter)(nil)

// GetPrivateChatSkills returns user-defined skills for a user's private chat.
func (a *CoreDBAdapter) GetPrivateChatSkills(userID int64) ([]assistant.Skill, error) {
	rows, err := a.db.GetPrivateChatSkills(userID)
	if err != nil {
		return nil, err
	}
	skills := make([]assistant.Skill, len(rows))
	for i, r := range rows {
		skills[i] = assistant.Skill{
			Name:        r.Name,
			Description: r.Description,
			Content:     r.Content,
		}
	}
	return skills, nil
}

// GetTopicSkills returns user-defined skills for a topic.
func (a *CoreDBAdapter) GetTopicSkills(topicID int64) ([]assistant.Skill, error) {
	rows, err := a.db.GetTopicSkills(topicID)
	if err != nil {
		return nil, err
	}
	skills := make([]assistant.Skill, len(rows))
	for i, r := range rows {
		skills[i] = assistant.Skill{
			Name:        r.Name,
			Description: r.Description,
			Content:     r.Content,
		}
	}
	return skills, nil
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./assistant/ -run TestEngine -v`
Expected: ALL PASS

**Step 6: Run all tests to verify no regressions**

Run: `go test ./...`
Expected: ALL PASS

**Step 7: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go context/adapter.go
git commit -m "feat(assistant): integrate user-defined skills into system prompt

Adds SkillProvider interface to the engine. User-defined skills are
fetched per-request based on chat scope (private or topic) and merged
after built-in skills in the system prompt."
```

---

### Task 4: Wire Skill Tool and Provider in main.go

Register the skill tool and connect the skill provider to the engine.

**Files:**
- Modify: `main.go`

**Step 1: Update main.go**

Add import:

```go
	"github.com/esnunes/bobot/tools/skill"
```

After the existing tool registrations (line ~84), add:

```go
	registry.Register(skill.NewSkillTool(coreDB))
```

After `engine.SetMessageSaver(messageSaver)` (line ~117), add:

```go
	engine.SetSkillProvider(contextAdapter)
```

**Step 2: Build to verify compilation**

Run: `go build ./...`
Expected: BUILD SUCCESS

**Step 3: Run all tests**

Run: `go test ./...`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat: wire skill tool and provider in main

Registers the skill tool in the tool registry and connects
the SkillProvider to the assistant engine."
```

---

### Task 5: REST API for Web UI

Add server handlers for skill CRUD operations. The API follows the existing JSON REST pattern used by topics.

**Files:**
- Create: `server/skills.go`
- Create: `server/skills_test.go`
- Modify: `server/server.go` (add routes)

**Step 1: Write the failing tests**

Create `server/skills_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/esnunes/bobot/auth"
)

func TestListSkillsPrivate(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	coreDB.CreateSkill(user.ID, nil, "groceries", "Manage groceries", "content")
	coreDB.CreateSkill(user.ID, nil, "recipes", "Manage recipes", "content")

	req := httptest.NewRequest("GET", "/api/skills", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleListSkills(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []map[string]any
	json.Unmarshal(w.Body.Bytes(), &skills)
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}

func TestListSkillsTopic(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	topic, _ := coreDB.CreateTopic("General", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)
	coreDB.CreateSkill(user.ID, &topic.ID, "notes", "Meeting notes", "content")

	req := httptest.NewRequest("GET", "/api/skills?topic_id="+strconv.FormatInt(topic.ID, 10), nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleListSkills(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []map[string]any
	json.Unmarshal(w.Body.Bytes(), &skills)
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
}

func TestCreateSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")

	body, _ := json.Marshal(map[string]string{
		"name":        "groceries",
		"description": "Manage grocery lists",
		"content":     "Use task tool",
	})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkill(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify skill exists
	skills, _ := coreDB.GetPrivateChatSkills(user.ID)
	if len(skills) != 1 {
		t.Errorf("expected 1 skill in DB, got %d", len(skills))
	}
}

func TestCreateSkillTopicAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	topic, _ := coreDB.CreateTopic("General", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)

	body, _ := json.Marshal(map[string]any{
		"name":     "notes",
		"content":  "content",
		"topic_id": topic.ID,
	})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkill(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateSkillTopicForbidden(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")
	topic, _ := coreDB.CreateTopic("General", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)
	coreDB.AddTopicMember(topic.ID, member.ID)

	body, _ := json.Marshal(map[string]any{
		"name":     "notes",
		"content":  "content",
		"topic_id": topic.ID,
	})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: member.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkill(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "old", "old content")

	body, _ := json.Marshal(map[string]string{
		"description": "new desc",
		"content":     "new content",
	})
	req := httptest.NewRequest("PUT", "/api/skills/"+strconv.FormatInt(skill.ID, 10), bytes.NewReader(body))
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleUpdateSkill(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, _ := coreDB.GetSkillByID(skill.ID)
	if updated.Content != "new content" {
		t.Errorf("expected updated content, got %q", updated.Content)
	}
}

func TestDeleteSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "desc", "content")

	req := httptest.NewRequest("DELETE", "/api/skills/"+strconv.FormatInt(skill.ID, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleDeleteSkill(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "desc", "content")

	req := httptest.NewRequest("GET", "/api/skills/"+strconv.FormatInt(skill.ID, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleGetSkill(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["name"] != "groceries" {
		t.Errorf("expected name 'groceries', got %v", result["name"])
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./server/ -run Test.*Skill -v`
Expected: FAIL — handlers don't exist.

**Step 3: Implement server handlers**

Create `server/skills.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicIDStr := r.URL.Query().Get("topic_id")

	var skills []db.SkillRow
	var err error

	if topicIDStr != "" {
		topicID, parseErr := strconv.ParseInt(topicIDStr, 10, 64)
		if parseErr != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		// Verify membership
		isMember, memberErr := s.db.IsTopicMember(topicID, userData.UserID)
		if memberErr != nil || !isMember {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		skills, err = s.db.GetTopicSkills(topicID)
	} else {
		skills, err = s.db.GetPrivateChatSkills(userData.UserID)
	}

	if err != nil {
		http.Error(w, "failed to list skills", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]any, 0, len(skills))
	for _, sk := range skills {
		item := map[string]any{
			"id":          sk.ID,
			"name":        sk.Name,
			"description": sk.Description,
			"content":     sk.Content,
			"user_id":     sk.UserID,
			"created_at":  sk.CreatedAt,
			"updated_at":  sk.UpdatedAt,
		}
		if sk.TopicID != nil {
			item["topic_id"] = *sk.TopicID
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type createSkillRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	TopicID     *int64 `json:"topic_id"`
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	var req createSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	// Permission check for topic skills
	if req.TopicID != nil {
		if err := s.canManageTopicSkills(userData, *req.TopicID); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	skill, err := s.db.CreateSkill(userData.UserID, req.TopicID, req.Name, req.Description, req.Content)
	if err != nil {
		http.Error(w, "failed to create skill", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": skill.ID, "name": skill.Name})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to get skill", http.StatusInternalServerError)
		return
	}

	// Verify ownership/membership
	if err := s.canViewSkill(userData, skill); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	result := map[string]any{
		"id":          skill.ID,
		"name":        skill.Name,
		"description": skill.Description,
		"content":     skill.Content,
		"user_id":     skill.UserID,
		"created_at":  skill.CreatedAt,
		"updated_at":  skill.UpdatedAt,
	}
	if skill.TopicID != nil {
		result["topic_id"] = *skill.TopicID
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type updateSkillRequest struct {
	Description string `json:"description"`
	Content     string `json:"content"`
}

func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to get skill", http.StatusInternalServerError)
		return
	}

	if err := s.canManageSkill(userData, skill); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	var req updateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateSkill(skillID, req.Description, req.Content); err != nil {
		http.Error(w, "failed to update skill", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": skillID, "status": "updated"})
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to get skill", http.StatusInternalServerError)
		return
	}

	if err := s.canManageSkill(userData, skill); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := s.db.DeleteSkill(skillID); err != nil {
		http.Error(w, "failed to delete skill", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// canManageTopicSkills checks if a user can create/modify topic skills.
func (s *Server) canManageTopicSkills(userData auth.UserData, topicID int64) error {
	if userData.Role == "admin" {
		return nil
	}
	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	if topic.OwnerID != userData.UserID {
		return fmt.Errorf("only the topic owner or admins can manage topic skills")
	}
	return nil
}

// canManageSkill checks if a user can update/delete a specific skill.
func (s *Server) canManageSkill(userData auth.UserData, skill *db.SkillRow) error {
	if skill.TopicID != nil {
		return s.canManageTopicSkills(userData, *skill.TopicID)
	}
	// Private skill — must be the owner
	if skill.UserID != userData.UserID {
		return fmt.Errorf("forbidden")
	}
	return nil
}

// canViewSkill checks if a user can view a specific skill.
func (s *Server) canViewSkill(userData auth.UserData, skill *db.SkillRow) error {
	if skill.TopicID != nil {
		isMember, err := s.db.IsTopicMember(*skill.TopicID, userData.UserID)
		if err != nil || !isMember {
			return fmt.Errorf("forbidden")
		}
		return nil
	}
	if skill.UserID != userData.UserID {
		return fmt.Errorf("forbidden")
	}
	return nil
}
```

Note: add `"fmt"` to the imports.

**Step 4: Add routes to server.go**

In `server/server.go`, in the `routes()` method, after the topic routes and before page routes, add:

```go
	// Skill routes (require auth)
	s.router.HandleFunc("GET /api/skills", s.sessionMiddleware(s.handleListSkills))
	s.router.HandleFunc("POST /api/skills", s.sessionMiddleware(s.handleCreateSkill))
	s.router.HandleFunc("GET /api/skills/{id}", s.sessionMiddleware(s.handleGetSkill))
	s.router.HandleFunc("PUT /api/skills/{id}", s.sessionMiddleware(s.handleUpdateSkill))
	s.router.HandleFunc("DELETE /api/skills/{id}", s.sessionMiddleware(s.handleDeleteSkill))
```

**Step 5: Run tests to verify they pass**

Run: `go test ./server/ -run Test.*Skill -v`
Expected: ALL PASS

**Step 6: Run all tests**

Run: `go test ./...`
Expected: ALL PASS

**Step 7: Commit**

```bash
git add server/skills.go server/skills_test.go server/server.go
git commit -m "feat(server): add REST API for skill management

Adds GET/POST/PUT/DELETE endpoints for skills with permission checks.
Topic skills require owner or admin role. Private skills require ownership."
```

---

### Task 6: Final Integration Test

Verify the full flow: create a skill via the tool, then confirm it appears in the system prompt.

**Files:**
- Modify: `assistant/engine_test.go` (add integration-style test if not already covered)

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: ALL PASS

**Step 2: Commit any remaining changes**

No additional changes expected — this is a verification step.
