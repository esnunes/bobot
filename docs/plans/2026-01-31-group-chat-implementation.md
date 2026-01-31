# Group Chat Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add group chat capability where users can create groups, manage membership, and chat with @assistant mentions.

**Architecture:** Extend existing database with groups/group_members tables, add nullable group_id to messages, modify WebSocket to include group_id in messages, add new API endpoints for group CRUD, create new frontend pages for groups.

**Tech Stack:** Go, SQLite, gorilla/websocket, HTML/CSS/JS templates

---

## Task 1: Database Schema - Groups Table

**Files:**
- Modify: `db/core.go`
- Test: `db/core_test.go`

**Step 1: Write failing test for CreateGroup**

Add to `db/core_test.go`:

```go
func TestCreateGroup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("owner", "hash")

	group, err := db.CreateGroup("Test Group", user.ID)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if group.ID == 0 {
		t.Error("expected non-zero group ID")
	}
	if group.Name != "Test Group" {
		t.Errorf("expected name 'Test Group', got %q", group.Name)
	}
	if group.OwnerID != user.ID {
		t.Errorf("expected owner_id %d, got %d", user.ID, group.OwnerID)
	}
	if group.DeletedAt != nil {
		t.Error("expected nil deleted_at for new group")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCreateGroup -v`
Expected: FAIL - Group type and CreateGroup method not defined

**Step 3: Add Group struct and migration**

Add to `db/core.go` after Invite struct:

```go
type Group struct {
	ID        int64
	Name      string
	OwnerID   int64
	DeletedAt *time.Time
	CreatedAt time.Time
}
```

Add to migrate() function after invites table:

```go
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
```

**Step 4: Add CreateGroup method**

Add to `db/core.go`:

```go
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
```

**Step 5: Run test to verify it passes**

Run: `go test ./db -run TestCreateGroup -v`
Expected: PASS

**Step 6: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add groups table and CreateGroup"
```

---

## Task 2: Database Schema - Group Members Table

**Files:**
- Modify: `db/core.go`
- Test: `db/core_test.go`

**Step 1: Write failing test for AddGroupMember**

Add to `db/core_test.go`:

```go
func TestAddGroupMember(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	err := db.AddGroupMember(group.ID, member.ID)
	if err != nil {
		t.Fatalf("AddGroupMember failed: %v", err)
	}

	// Adding same member again should fail or be idempotent
	err = db.AddGroupMember(group.ID, member.ID)
	// SQLite will error on duplicate primary key
	if err == nil {
		t.Error("expected error when adding duplicate member")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestAddGroupMember -v`
Expected: FAIL - AddGroupMember not defined

**Step 3: Add migration for group_members table**

Add to migrate() after groups table:

```go
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
```

**Step 4: Add AddGroupMember method**

Add to `db/core.go`:

```go
func (c *CoreDB) AddGroupMember(groupID, userID int64) error {
	_, err := c.db.Exec(
		"INSERT INTO group_members (group_id, user_id) VALUES (?, ?)",
		groupID, userID,
	)
	return err
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./db -run TestAddGroupMember -v`
Expected: PASS

**Step 6: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add group_members table and AddGroupMember"
```

---

## Task 3: Database - Remove Group Member

**Files:**
- Modify: `db/core.go`
- Test: `db/core_test.go`

**Step 1: Write failing test**

Add to `db/core_test.go`:

```go
func TestRemoveGroupMember(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)
	db.AddGroupMember(group.ID, member.ID)

	err := db.RemoveGroupMember(group.ID, member.ID)
	if err != nil {
		t.Fatalf("RemoveGroupMember failed: %v", err)
	}

	// Verify member is removed by checking membership
	isMember, _ := db.IsGroupMember(group.ID, member.ID)
	if isMember {
		t.Error("expected member to be removed")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestRemoveGroupMember -v`
Expected: FAIL - RemoveGroupMember and IsGroupMember not defined

**Step 3: Add RemoveGroupMember and IsGroupMember methods**

Add to `db/core.go`:

```go
func (c *CoreDB) RemoveGroupMember(groupID, userID int64) error {
	_, err := c.db.Exec(
		"DELETE FROM group_members WHERE group_id = ? AND user_id = ?",
		groupID, userID,
	)
	return err
}

func (c *CoreDB) IsGroupMember(groupID, userID int64) (bool, error) {
	var count int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM group_members WHERE group_id = ? AND user_id = ?",
		groupID, userID,
	).Scan(&count)
	return count > 0, err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run TestRemoveGroupMember -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add RemoveGroupMember and IsGroupMember"
```

---

## Task 4: Database - Get Group and List User Groups

**Files:**
- Modify: `db/core.go`
- Test: `db/core_test.go`

**Step 1: Write failing tests**

Add to `db/core_test.go`:

```go
func TestGetGroupByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	created, _ := db.CreateGroup("Test Group", owner.ID)

	group, err := db.GetGroupByID(created.ID)
	if err != nil {
		t.Fatalf("GetGroupByID failed: %v", err)
	}
	if group.Name != "Test Group" {
		t.Errorf("expected name 'Test Group', got %q", group.Name)
	}

	// Test not found
	_, err = db.GetGroupByID(9999)
	if err != db.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetUserGroups(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")

	group1, _ := db.CreateGroup("Group 1", owner.ID)
	group2, _ := db.CreateGroup("Group 2", owner.ID)
	db.AddGroupMember(group1.ID, member.ID)
	db.AddGroupMember(group2.ID, member.ID)

	groups, err := db.GetUserGroups(member.ID)
	if err != nil {
		t.Fatalf("GetUserGroups failed: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./db -run "TestGetGroupByID|TestGetUserGroups" -v`
Expected: FAIL

**Step 3: Add GetGroupByID and GetUserGroups methods**

Add to `db/core.go`:

```go
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./db -run "TestGetGroupByID|TestGetUserGroups" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add GetGroupByID and GetUserGroups"
```

---

## Task 5: Database - Soft Delete Group and Get Group Members

**Files:**
- Modify: `db/core.go`
- Test: `db/core_test.go`

**Step 1: Write failing tests**

Add to `db/core_test.go`:

```go
func TestSoftDeleteGroup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	err := db.SoftDeleteGroup(group.ID)
	if err != nil {
		t.Fatalf("SoftDeleteGroup failed: %v", err)
	}

	// Should not be found after soft delete
	_, err = db.GetGroupByID(group.ID)
	if err != db.ErrNotFound {
		t.Errorf("expected ErrNotFound after soft delete, got %v", err)
	}
}

func TestGetGroupMembers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUserFull("owner", "hash", "Owner", "user")
	member, _ := db.CreateUserFull("member", "hash", "Member", "user")
	group, _ := db.CreateGroup("Test Group", owner.ID)
	db.AddGroupMember(group.ID, owner.ID)
	db.AddGroupMember(group.ID, member.ID)

	members, err := db.GetGroupMembers(group.ID)
	if err != nil {
		t.Fatalf("GetGroupMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./db -run "TestSoftDeleteGroup|TestGetGroupMembers" -v`
Expected: FAIL

**Step 3: Add SoftDeleteGroup and GetGroupMembers methods**

Add to `db/core.go`:

```go
func (c *CoreDB) SoftDeleteGroup(groupID int64) error {
	_, err := c.db.Exec(
		"UPDATE groups SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?",
		groupID,
	)
	return err
}

type GroupMember struct {
	UserID      int64
	Username    string
	DisplayName string
	JoinedAt    time.Time
}

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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./db -run "TestSoftDeleteGroup|TestGetGroupMembers" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add SoftDeleteGroup and GetGroupMembers"
```

---

## Task 6: Database - Add group_id to Messages

**Files:**
- Modify: `db/core.go`
- Test: `db/core_test.go`

**Step 1: Write failing test**

Add to `db/core_test.go`:

```go
func TestCreateGroupMessage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	msg, err := db.CreateGroupMessage(group.ID, owner.ID, "user", "Hello group!")
	if err != nil {
		t.Fatalf("CreateGroupMessage failed: %v", err)
	}
	if msg.GroupID == nil || *msg.GroupID != group.ID {
		t.Error("expected group_id to be set")
	}
	if msg.Content != "Hello group!" {
		t.Errorf("expected content 'Hello group!', got %q", msg.Content)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCreateGroupMessage -v`
Expected: FAIL

**Step 3: Update Message struct and add migration**

Modify Message struct in `db/core.go`:

```go
type Message struct {
	ID            int64
	UserID        int64
	GroupID       *int64  // nil for 1:1 chats, set for group messages
	Role          string
	Content       string
	Tokens        int
	ContextTokens int
	CreatedAt     time.Time
}
```

Add migration in migrate() after existing message migrations:

```go
// Migrate: add group_id column to messages
if err := c.addColumnIfMissing("messages", "group_id", "INTEGER REFERENCES groups(id)"); err != nil {
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
```

**Step 4: Add CreateGroupMessage method**

Add to `db/core.go`:

```go
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
```

**Step 5: Update existing message scan functions to handle GroupID**

Update all Scan calls in message-related functions to include group_id. For example, update GetRecentMessages:

```go
func (c *CoreDB) GetRecentMessages(userID int64, limit int) ([]Message, error) {
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
```

Apply similar changes to: GetMessages, GetContextMessages, GetMessagesBefore, GetMessagesSince, CreateMessage, CreateMessageWithContext, CreateMessageWithContextThreshold.

**Step 6: Run test to verify it passes**

Run: `go test ./db -run TestCreateGroupMessage -v`
Expected: PASS

**Step 7: Run all db tests to ensure no regressions**

Run: `go test ./db -v`
Expected: All tests PASS

**Step 8: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add group_id to messages table"
```

---

## Task 7: Database - Group Message Queries

**Files:**
- Modify: `db/core.go`
- Test: `db/core_test.go`

**Step 1: Write failing tests**

Add to `db/core_test.go`:

```go
func TestGetGroupRecentMessages(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	db.CreateGroupMessage(group.ID, owner.ID, "user", "Message 1")
	db.CreateGroupMessage(group.ID, owner.ID, "assistant", "Response 1")
	db.CreateGroupMessage(group.ID, owner.ID, "user", "Message 2")

	msgs, err := db.GetGroupRecentMessages(group.ID, 10)
	if err != nil {
		t.Fatalf("GetGroupRecentMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
}

func TestGetGroupContextMessages(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	db.CreateGroupMessageWithContext(group.ID, owner.ID, "user", "Hello", 1000, 80000)
	db.CreateGroupMessageWithContext(group.ID, owner.ID, "assistant", "Hi there", 1000, 80000)

	msgs, err := db.GetGroupContextMessages(group.ID)
	if err != nil {
		t.Fatalf("GetGroupContextMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 context messages, got %d", len(msgs))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./db -run "TestGetGroupRecentMessages|TestGetGroupContextMessages" -v`
Expected: FAIL

**Step 3: Add group message query methods**

Add to `db/core.go`:

```go
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./db -run "TestGetGroupRecentMessages|TestGetGroupContextMessages" -v`
Expected: PASS

**Step 5: Run all db tests**

Run: `go test ./db -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add group message query methods"
```

---

## Task 8: Server - Group API Endpoints (Create and List)

**Files:**
- Create: `server/groups.go`
- Test: `server/groups_test.go`
- Modify: `server/server.go`

**Step 1: Write failing test for create group**

Create `server/groups_test.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func setupGroupTestServer(t *testing.T) (*Server, *db.CoreDB, func()) {
	coreDB, err := db.NewCoreDB(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	jwt := auth.NewJWTService("test-secret-key-32-chars-min!!")
	cfg := &config.Config{}
	s := New(cfg, coreDB, jwt)

	return s, coreDB, func() { coreDB.Close() }
}

func TestCreateGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(user.ID, "user")

	body := `{"name": "My Group"}`
	req := httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["name"] != "My Group" {
		t.Errorf("expected name 'My Group', got %v", resp["name"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestCreateGroup -v`
Expected: FAIL - route not found

**Step 3: Create groups.go with handlers**

Create `server/groups.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
)

type createGroupRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		http.Error(w, "name required (max 100 chars)", http.StatusBadRequest)
		return
	}

	group, err := s.db.CreateGroup(req.Name, userData.UserID)
	if err != nil {
		http.Error(w, "failed to create group", http.StatusInternalServerError)
		return
	}

	// Add creator as first member
	if err := s.db.AddGroupMember(group.ID, userData.UserID); err != nil {
		http.Error(w, "failed to add owner to group", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       group.ID,
		"name":     group.Name,
		"owner_id": group.OwnerID,
	})
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groups, err := s.db.GetUserGroups(userData.UserID)
	if err != nil {
		http.Error(w, "failed to list groups", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		members, _ := s.db.GetGroupMembers(g.ID)
		result = append(result, map[string]interface{}{
			"id":           g.ID,
			"name":         g.Name,
			"owner_id":     g.OwnerID,
			"member_count": len(members),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
```

**Step 4: Add routes to server.go**

Add to routes() in `server/server.go`:

```go
// Group routes (require auth)
s.router.HandleFunc("POST /api/groups", s.authMiddleware(s.handleCreateGroup))
s.router.HandleFunc("GET /api/groups", s.authMiddleware(s.handleListGroups))
```

**Step 5: Run test to verify it passes**

Run: `go test ./server -run TestCreateGroup -v`
Expected: PASS

**Step 6: Write and run test for list groups**

Add to `server/groups_test.go`:

```go
func TestListGroups(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(user.ID, "user")

	// Create a group first
	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)

	req := httptest.NewRequest("GET", "/api/groups", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var groups []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &groups)
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
}
```

Run: `go test ./server -run TestListGroups -v`
Expected: PASS

**Step 7: Commit**

```bash
git add server/groups.go server/groups_test.go server/server.go
git commit -m "feat(server): add create and list group endpoints"
```

---

## Task 9: Server - Group Detail and Delete Endpoints

**Files:**
- Modify: `server/groups.go`
- Test: `server/groups_test.go`
- Modify: `server/server.go`

**Step 1: Write failing tests**

Add to `server/groups_test.go`:

```go
func TestGetGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(user.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)

	req := httptest.NewRequest("GET", "/api/groups/"+strconv.FormatInt(group.ID, 10), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(user.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/"+strconv.FormatInt(group.ID, 10), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify soft deleted
	_, err := coreDB.GetGroupByID(group.ID)
	if err != db.ErrNotFound {
		t.Error("expected group to be soft deleted")
	}
}

func TestDeleteGroupNotOwner(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")
	memberToken, _ := s.jwt.GenerateAccessTokenWithRole(member.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)
	coreDB.AddGroupMember(group.ID, member.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/"+strconv.FormatInt(group.ID, 10), nil)
	req.Header.Set("Authorization", "Bearer "+memberToken)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}
```

Add import for strconv and db at top of test file.

**Step 2: Run tests to verify they fail**

Run: `go test ./server -run "TestGetGroup|TestDeleteGroup" -v`
Expected: FAIL

**Step 3: Add handlers**

Add to `server/groups.go`:

```go
func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	// Extract group ID from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	// Check membership
	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	members, _ := s.db.GetGroupMembers(groupID)

	memberList := make([]map[string]interface{}, 0, len(members))
	for _, m := range members {
		memberList = append(memberList, map[string]interface{}{
			"user_id":      m.UserID,
			"username":     m.Username,
			"display_name": m.DisplayName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       group.ID,
		"name":     group.Name,
		"owner_id": group.OwnerID,
		"members":  memberList,
	})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	// Only owner can delete
	if group.OwnerID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.SoftDeleteGroup(groupID); err != nil {
		http.Error(w, "failed to delete group", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

**Step 4: Add routes**

Add to routes() in `server/server.go`:

```go
s.router.HandleFunc("GET /api/groups/{id}", s.authMiddleware(s.handleGetGroup))
s.router.HandleFunc("DELETE /api/groups/{id}", s.authMiddleware(s.handleDeleteGroup))
```

Note: Go 1.22+ supports path parameters with {id}. If using older version, use manual path parsing.

**Step 5: Run tests**

Run: `go test ./server -run "TestGetGroup|TestDeleteGroup" -v`
Expected: PASS

**Step 6: Commit**

```bash
git add server/groups.go server/groups_test.go server/server.go
git commit -m "feat(server): add get and delete group endpoints"
```

---

## Task 10: Server - Group Member Endpoints

**Files:**
- Modify: `server/groups.go`
- Test: `server/groups_test.go`
- Modify: `server/server.go`

**Step 1: Write failing tests**

Add to `server/groups_test.go`:

```go
func TestAddGroupMember(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	newMember, _ := coreDB.CreateUser("newmember", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(owner.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)

	body := `{"username": "newmember"}`
	req := httptest.NewRequest("POST", "/api/groups/"+strconv.FormatInt(group.ID, 10)+"/members", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify member was added
	isMember, _ := coreDB.IsGroupMember(group.ID, newMember.ID)
	if !isMember {
		t.Error("expected new member to be added")
	}
}

func TestRemoveGroupMember(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(owner.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)
	coreDB.AddGroupMember(group.ID, member.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/"+strconv.FormatInt(group.ID, 10)+"/members/"+strconv.FormatInt(member.ID, 10), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLeaveGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")
	memberToken, _ := s.jwt.GenerateAccessTokenWithRole(member.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)
	coreDB.AddGroupMember(group.ID, member.ID)

	// Member removes self
	req := httptest.NewRequest("DELETE", "/api/groups/"+strconv.FormatInt(group.ID, 10)+"/members/"+strconv.FormatInt(member.ID, 10), nil)
	req.Header.Set("Authorization", "Bearer "+memberToken)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}

func TestOwnerCannotLeave(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(owner.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/"+strconv.FormatInt(group.ID, 10)+"/members/"+strconv.FormatInt(owner.ID, 10), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./server -run "TestAddGroupMember|TestRemoveGroupMember|TestLeaveGroup|TestOwnerCannotLeave" -v`
Expected: FAIL

**Step 3: Add handlers**

Add to `server/groups.go`:

```go
type addMemberRequest struct {
	Username string `json:"username"`
}

func (s *Server) handleAddGroupMember(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	// Only owner can add members
	if group.OwnerID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, err := s.db.GetUserByUsername(req.Username)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	if err := s.db.AddGroupMember(groupID, user.ID); err != nil {
		// Could be duplicate - treat as success (idempotent)
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "failed to add member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}
	targetUserID, err := strconv.ParseInt(parts[5], 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	// Owner cannot leave (must delete group)
	if targetUserID == group.OwnerID {
		http.Error(w, "owner cannot leave group", http.StatusForbidden)
		return
	}

	// Only owner or self can remove
	if group.OwnerID != userData.UserID && targetUserID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.RemoveGroupMember(groupID, targetUserID); err != nil {
		http.Error(w, "failed to remove member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

**Step 4: Add routes**

Add to routes() in `server/server.go`:

```go
s.router.HandleFunc("POST /api/groups/{id}/members", s.authMiddleware(s.handleAddGroupMember))
s.router.HandleFunc("DELETE /api/groups/{id}/members/{userId}", s.authMiddleware(s.handleRemoveGroupMember))
```

**Step 5: Run tests**

Run: `go test ./server -run "TestAddGroupMember|TestRemoveGroupMember|TestLeaveGroup|TestOwnerCannotLeave" -v`
Expected: PASS

**Step 6: Commit**

```bash
git add server/groups.go server/groups_test.go server/server.go
git commit -m "feat(server): add group member management endpoints"
```

---

## Task 11: Server - Group Message Endpoints

**Files:**
- Modify: `server/groups.go`
- Test: `server/groups_test.go`
- Modify: `server/server.go`

**Step 1: Write failing test**

Add to `server/groups_test.go`:

```go
func TestGetGroupMessages(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")
	token, _ := s.jwt.GenerateAccessTokenWithRole(user.ID, "user")

	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)
	coreDB.CreateGroupMessage(group.ID, user.ID, "user", "Hello")
	coreDB.CreateGroupMessage(group.ID, user.ID, "assistant", "Hi there")

	req := httptest.NewRequest("GET", "/api/groups/"+strconv.FormatInt(group.ID, 10)+"/messages/recent?limit=50", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var messages []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &messages)
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestGetGroupMessages -v`
Expected: FAIL

**Step 3: Add handlers**

Add to `server/groups.go`:

```go
func (s *Server) handleGroupRecentMessages(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	// Check membership
	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	messages, err := s.db.GetGroupRecentMessages(groupID, limit)
	if err != nil {
		http.Error(w, "failed to get messages", http.StatusInternalServerError)
		return
	}

	// Enrich with user display names
	result := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		item := map[string]interface{}{
			"ID":        m.ID,
			"Role":      m.Role,
			"Content":   m.Content,
			"CreatedAt": m.CreatedAt,
		}
		if m.Role == "user" {
			if user, err := s.db.GetUserByID(m.UserID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGroupMessageHistory(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	messages, err := s.db.GetGroupMessagesBefore(groupID, beforeID, limit)
	if err != nil {
		http.Error(w, "failed to get messages", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		item := map[string]interface{}{
			"ID":        m.ID,
			"Role":      m.Role,
			"Content":   m.Content,
			"CreatedAt": m.CreatedAt,
		}
		if m.Role == "user" {
			if user, err := s.db.GetUserByID(m.UserID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGroupMessageSync(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	since, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil {
		http.Error(w, "invalid since parameter", http.StatusBadRequest)
		return
	}

	messages, err := s.db.GetGroupMessagesSince(groupID, since)
	if err != nil {
		http.Error(w, "failed to get messages", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		item := map[string]interface{}{
			"ID":        m.ID,
			"Role":      m.Role,
			"Content":   m.Content,
			"CreatedAt": m.CreatedAt,
		}
		if m.Role == "user" {
			if user, err := s.db.GetUserByID(m.UserID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
```

Add `"time"` to imports.

**Step 4: Add routes**

Add to routes() in `server/server.go`:

```go
s.router.HandleFunc("GET /api/groups/{id}/messages/recent", s.authMiddleware(s.handleGroupRecentMessages))
s.router.HandleFunc("GET /api/groups/{id}/messages/history", s.authMiddleware(s.handleGroupMessageHistory))
s.router.HandleFunc("GET /api/groups/{id}/messages/sync", s.authMiddleware(s.handleGroupMessageSync))
```

**Step 5: Run test**

Run: `go test ./server -run TestGetGroupMessages -v`
Expected: PASS

**Step 6: Commit**

```bash
git add server/groups.go server/groups_test.go server/server.go
git commit -m "feat(server): add group message endpoints"
```

---

## Task 12: WebSocket - Add group_id to Message Format

**Files:**
- Modify: `server/chat.go`
- Test: `server/chat_test.go`

**Step 1: Write failing test**

Add to `server/chat_test.go` (create if needed):

```go
func TestGroupMessage(t *testing.T) {
	// This test verifies the message struct accepts group_id
	msg := chatMessage{
		Content: "Hello",
		GroupID: ptr(int64(5)),
	}
	if msg.GroupID == nil || *msg.GroupID != 5 {
		t.Error("expected group_id to be 5")
	}
}

func ptr[T any](v T) *T {
	return &v
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestGroupMessage -v`
Expected: FAIL - chatMessage has no GroupID field

**Step 3: Update chatMessage struct**

Modify in `server/chat.go`:

```go
type chatMessage struct {
	Content string `json:"content"`
	GroupID *int64 `json:"group_id"`
}
```

**Step 4: Run test**

Run: `go test ./server -run TestGroupMessage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/chat.go server/chat_test.go
git commit -m "feat(server): add group_id to websocket message format"
```

---

## Task 13: WebSocket - Handle Group Messages

**Files:**
- Modify: `server/chat.go`

**Step 1: Update handleChat to process group messages**

Replace the message processing loop in `server/chat.go` handleChat function:

```go
// Handle messages
for {
	var msg chatMessage
	if err := conn.ReadJSON(&msg); err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			log.Printf("websocket error: %v", err)
		}
		break
	}

	if msg.GroupID != nil {
		// Handle group message
		s.handleGroupChatMessage(ctx, claims.UserID, *msg.GroupID, msg.Content)
	} else {
		// Handle 1:1 message (existing logic)
		s.handlePrivateChatMessage(ctx, claims.UserID, msg.Content)
	}
}
```

**Step 2: Extract existing logic into handlePrivateChatMessage**

Add to `server/chat.go`:

```go
func (s *Server) handlePrivateChatMessage(ctx context.Context, userID int64, content string) {
	// Check for slash commands
	if response, handled := s.handleSlashCommand(ctx, content); handled {
		s.db.CreateMessageWithContextThreshold(
			userID, "command", content,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		userMsgJSON, _ := json.Marshal(map[string]interface{}{
			"role":    "command",
			"content": content,
		})
		s.connections.Broadcast(userID, userMsgJSON)

		s.db.CreateMessageWithContextThreshold(
			userID, "system", response,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		respJSON, _ := json.Marshal(map[string]interface{}{
			"role":    "system",
			"content": response,
		})
		s.connections.Broadcast(userID, respJSON)
		return
	}

	// Save user message with context tracking
	s.db.CreateMessageWithContextThreshold(
		userID, "user", content,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast user message
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "user",
		"content": content,
	})
	s.connections.Broadcast(userID, userMsgJSON)

	// Get assistant response
	response, err := s.engine.Chat(ctx, content)
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Save assistant message
	s.db.CreateMessageWithContextThreshold(
		userID, "assistant", response,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast assistant response
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "assistant",
		"content": response,
	})
	s.connections.Broadcast(userID, assistantMsgJSON)
}
```

**Step 3: Add handleGroupChatMessage**

Add to `server/chat.go`:

```go
func (s *Server) handleGroupChatMessage(ctx context.Context, userID, groupID int64, content string) {
	// Verify membership
	isMember, err := s.db.IsGroupMember(groupID, userID)
	if err != nil || !isMember {
		log.Printf("user %d not member of group %d", userID, groupID)
		return
	}

	// Get user info for display
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		log.Printf("failed to get user: %v", err)
		return
	}

	// Save user message
	s.db.CreateGroupMessageWithContext(
		groupID, userID, "user", content,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast to all group members
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"group_id":     groupID,
		"role":         "user",
		"content":      content,
		"user_id":      userID,
		"display_name": user.DisplayName,
	})
	s.broadcastToGroup(groupID, userMsgJSON)

	// Check if assistant should respond
	if shouldTriggerAssistant(content) {
		s.handleGroupAssistantResponse(ctx, groupID)
	}
}

func shouldTriggerAssistant(content string) bool {
	return strings.Contains(strings.ToLower(content), "@assistant")
}

func (s *Server) handleGroupAssistantResponse(ctx context.Context, groupID int64) {
	// Get context messages
	messages, err := s.db.GetGroupContextMessages(groupID)
	if err != nil {
		log.Printf("failed to get group context: %v", err)
		return
	}

	// Build conversation for LLM with user attribution
	var conversation []string
	for _, m := range messages {
		if m.Role == "user" {
			user, _ := s.db.GetUserByID(m.UserID)
			name := "User"
			if user != nil && user.DisplayName != "" {
				name = user.DisplayName
			}
			conversation = append(conversation, fmt.Sprintf("[%s]: %s", name, m.Content))
		} else {
			conversation = append(conversation, fmt.Sprintf("[assistant]: %s", m.Content))
		}
	}

	// Get response from engine
	response, err := s.engine.ChatWithContext(ctx, conversation)
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Save assistant message
	s.db.CreateGroupMessageWithContext(
		groupID, 0, "assistant", response,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast to group
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"group_id": groupID,
		"role":     "assistant",
		"content":  response,
	})
	s.broadcastToGroup(groupID, assistantMsgJSON)
}

func (s *Server) broadcastToGroup(groupID int64, data []byte) {
	members, err := s.db.GetGroupMembers(groupID)
	if err != nil {
		log.Printf("failed to get group members: %v", err)
		return
	}

	for _, member := range members {
		s.connections.Broadcast(member.UserID, data)
	}
}
```

Add `"fmt"` to imports.

**Step 4: Run all server tests**

Run: `go test ./server -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/chat.go
git commit -m "feat(server): handle group messages via websocket"
```

---

## Task 14: Assistant Engine - Group Context Support

**Files:**
- Modify: `assistant/engine.go`
- Test: `assistant/engine_test.go`

**Step 1: Write failing test**

Add to `assistant/engine_test.go`:

```go
func TestChatWithContext(t *testing.T) {
	mockProvider := &mockLLMProvider{
		response: "Test response",
	}
	engine := New(mockProvider, nil, nil)

	conversation := []string{
		"[Alice]: Hello @assistant",
		"[Bob]: Yes, please help us",
	}

	response, err := engine.ChatWithContext(context.Background(), conversation)
	if err != nil {
		t.Fatalf("ChatWithContext failed: %v", err)
	}
	if response == "" {
		t.Error("expected non-empty response")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./assistant -run TestChatWithContext -v`
Expected: FAIL - ChatWithContext not defined

**Step 3: Add ChatWithContext method**

Add to `assistant/engine.go`:

```go
func (e *Engine) ChatWithContext(ctx context.Context, conversation []string) (string, error) {
	// Build messages from conversation
	var messages []llm.Message

	// Add system prompt for group context
	systemPrompt := `You are a helpful AI assistant participating in a group chat.
Messages are formatted as [Name]: message content.
Only respond when specifically addressed with @assistant.
Keep responses concise and relevant to the conversation.`

	messages = append(messages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	// Add conversation history
	for _, line := range conversation {
		if strings.HasPrefix(line, "[assistant]:") {
			content := strings.TrimPrefix(line, "[assistant]: ")
			messages = append(messages, llm.Message{
				Role:    "assistant",
				Content: content,
			})
		} else {
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: line,
			})
		}
	}

	return e.provider.Chat(ctx, messages)
}
```

**Step 4: Run test**

Run: `go test ./assistant -run TestChatWithContext -v`
Expected: PASS

**Step 5: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go
git commit -m "feat(assistant): add ChatWithContext for group conversations"
```

---

## Task 15: Frontend - Group List Page Template

**Files:**
- Create: `web/templates/groups.html`
- Modify: `server/pages.go`
- Modify: `server/server.go`

**Step 1: Create groups.html template**

Create `web/templates/groups.html`:

```html
{{define "content"}}
<div class="groups-container">
    <header class="groups-header">
        <a href="/chat" class="back-link">&larr; Chat</a>
        <h1>Groups</h1>
        <button id="create-group-btn" class="create-btn">+</button>
    </header>

    <main class="groups-list" id="groups-list">
        <div class="loading">Loading groups...</div>
    </main>
</div>

<div id="create-group-modal" class="modal hidden">
    <div class="modal-content">
        <h2>Create Group</h2>
        <form id="create-group-form">
            <input type="text" id="group-name" placeholder="Group name" required maxlength="100">
            <div class="modal-actions">
                <button type="button" id="cancel-create">Cancel</button>
                <button type="submit">Create</button>
            </div>
        </form>
    </div>
</div>

<script>
class GroupsPage {
    constructor() {
        this.listEl = document.getElementById('groups-list');
        this.modal = document.getElementById('create-group-modal');
        this.form = document.getElementById('create-group-form');
        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            window.location.href = '/';
            return;
        }

        await this.loadGroups(token);
        this.setupEventListeners();
    }

    async loadGroups(token) {
        try {
            const resp = await fetch('/api/groups', {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 401) {
                    window.location.href = '/';
                    return;
                }
                throw new Error('Failed to load groups');
            }

            const groups = await resp.json();
            this.renderGroups(groups);
        } catch (err) {
            console.error('Failed to load groups:', err);
            this.listEl.innerHTML = '<div class="error">Failed to load groups</div>';
        }
    }

    renderGroups(groups) {
        if (!groups || groups.length === 0) {
            this.listEl.innerHTML = '<div class="empty">No groups yet. Create one!</div>';
            return;
        }

        this.listEl.innerHTML = groups.map(g => `
            <a href="/groups/${g.id}" class="group-item">
                <span class="group-name">${this.escapeHtml(g.name)}</span>
                <span class="group-members">${g.member_count} member${g.member_count !== 1 ? 's' : ''}</span>
            </a>
        `).join('');
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    setupEventListeners() {
        document.getElementById('create-group-btn').addEventListener('click', () => {
            this.modal.classList.remove('hidden');
        });

        document.getElementById('cancel-create').addEventListener('click', () => {
            this.modal.classList.add('hidden');
        });

        this.modal.addEventListener('click', (e) => {
            if (e.target === this.modal) {
                this.modal.classList.add('hidden');
            }
        });

        this.form.addEventListener('submit', async (e) => {
            e.preventDefault();
            await this.createGroup();
        });
    }

    async createGroup() {
        const name = document.getElementById('group-name').value.trim();
        if (!name) return;

        const token = localStorage.getItem('access_token');
        try {
            const resp = await fetch('/api/groups', {
                method: 'POST',
                headers: {
                    'Authorization': `Bearer ${token}`,
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ name })
            });

            if (!resp.ok) throw new Error('Failed to create group');

            const group = await resp.json();
            window.location.href = `/groups/${group.id}`;
        } catch (err) {
            console.error('Failed to create group:', err);
            alert('Failed to create group');
        }
    }
}

document.addEventListener('DOMContentLoaded', () => new GroupsPage());
</script>
{{end}}
```

**Step 2: Add page handler**

Add to `server/pages.go`:

```go
func (s *Server) handleGroupsPage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "groups", map[string]interface{}{
		"Title": "Groups",
	})
}
```

**Step 3: Add route**

Add to routes() in `server/server.go`:

```go
s.router.HandleFunc("GET /groups", s.handleGroupsPage)
```

**Step 4: Load template**

Update loadTemplates() in `server/pages.go` to include groups template.

**Step 5: Test manually**

Run: `go build && ./bobot`
Navigate to `/groups`
Expected: Page loads without error

**Step 6: Commit**

```bash
git add web/templates/groups.html server/pages.go server/server.go
git commit -m "feat(web): add groups list page"
```

---

## Task 16: Frontend - Group Chat Page Template

**Files:**
- Create: `web/templates/group_chat.html`
- Create: `web/static/group_chat.js`
- Modify: `server/pages.go`
- Modify: `server/server.go`

**Step 1: Create group_chat.html**

Create `web/templates/group_chat.html`:

```html
{{define "content"}}
<div class="chat-container">
    <header class="chat-header">
        <a href="/groups" class="back-link">&larr;</a>
        <h1 id="group-name">Loading...</h1>
        <button id="menu-btn" class="menu-btn">&#9776;</button>
    </header>

    <main class="chat-messages" id="messages">
    </main>

    <footer class="chat-input">
        <form id="chat-form">
            <input type="text" id="message-input" placeholder="Type @assistant to ask..." autocomplete="off">
            <button type="submit">&#10148;</button>
        </form>
    </footer>
</div>

<div id="menu-overlay" class="menu-overlay hidden">
    <div class="menu">
        <div id="members-list" class="members-list"></div>
        <hr>
        <button id="leave-btn" class="danger-btn">Leave Group</button>
        <button id="delete-btn" class="danger-btn hidden">Delete Group</button>
    </div>
</div>

<script>
const GROUP_ID = {{.GroupID}};
</script>
<script src="/static/group_chat.js"></script>
{{end}}
```

**Step 2: Create group_chat.js**

Create `web/static/group_chat.js`:

```javascript
class GroupChatClient {
    constructor(groupId) {
        this.groupId = groupId;
        this.ws = null;
        this.messagesEl = document.getElementById('messages');
        this.form = document.getElementById('chat-form');
        this.input = document.getElementById('message-input');
        this.menuBtn = document.getElementById('menu-btn');
        this.menuOverlay = document.getElementById('menu-overlay');
        this.leaveBtn = document.getElementById('leave-btn');
        this.deleteBtn = document.getElementById('delete-btn');
        this.isLoadingHistory = false;
        this.oldestMessageId = null;
        this.hasMoreHistory = true;
        this.currentUserId = null;

        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            window.location.href = '/';
            return;
        }

        await this.loadGroupInfo(token);
        await this.loadRecentMessages(token);
        this.connect(token);
        this.setupEventListeners();
    }

    async loadGroupInfo(token) {
        try {
            const resp = await fetch(`/api/groups/${this.groupId}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 403 || resp.status === 404) {
                    window.location.href = '/groups';
                    return;
                }
                throw new Error('Failed to load group');
            }

            const group = await resp.json();
            document.getElementById('group-name').textContent = group.name;

            // Parse JWT to get current user ID
            const payload = JSON.parse(atob(token.split('.')[1]));
            this.currentUserId = payload.user_id;

            // Show delete button if owner
            if (group.owner_id === this.currentUserId) {
                this.deleteBtn.classList.remove('hidden');
                this.leaveBtn.classList.add('hidden');
            }

            // Render members
            const membersList = document.getElementById('members-list');
            membersList.innerHTML = '<strong>Members:</strong>' + group.members.map(m =>
                `<div class="member">${this.escapeHtml(m.display_name || m.username)}</div>`
            ).join('');

        } catch (err) {
            console.error('Failed to load group:', err);
        }
    }

    async loadRecentMessages(token) {
        try {
            const resp = await fetch(`/api/groups/${this.groupId}/messages/recent?limit=50`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to load messages');

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                messages.forEach(msg => this.addMessage(msg, false));
                this.oldestMessageId = messages[0].ID;
            }
            this.scrollToBottom();
        } catch (err) {
            console.error('Failed to load messages:', err);
        }
    }

    connect(token) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat?token=${token}`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => console.log('WebSocket connected');

        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            // Only handle messages for this group
            if (data.group_id === this.groupId) {
                if (data.role === 'assistant') {
                    this.removeTypingIndicator();
                }
                this.addMessage(data, true);
            }
        };

        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            setTimeout(() => this.reconnect(), 1000);
        };
    }

    async reconnect() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
            window.location.href = '/';
            return;
        }

        try {
            const resp = await fetch('/api/refresh', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken })
            });

            if (!resp.ok) throw new Error('Refresh failed');

            const data = await resp.json();
            localStorage.setItem('access_token', data.access_token);
            this.connect(data.access_token);
        } catch (err) {
            console.error('Reconnect failed:', err);
            window.location.href = '/';
        }
    }

    setupEventListeners() {
        this.form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.sendMessage();
        });

        this.menuBtn.addEventListener('click', () => {
            this.menuOverlay.classList.remove('hidden');
        });

        this.menuOverlay.addEventListener('click', (e) => {
            if (e.target === this.menuOverlay) {
                this.menuOverlay.classList.add('hidden');
            }
        });

        this.leaveBtn.addEventListener('click', () => this.leaveGroup());
        this.deleteBtn.addEventListener('click', () => this.deleteGroup());

        this.messagesEl.addEventListener('scroll', () => {
            if (this.messagesEl.scrollTop < 100) {
                this.loadMoreHistory();
            }
        });
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN) return;

        this.ws.send(JSON.stringify({
            content: content,
            group_id: this.groupId
        }));
        this.input.value = '';

        if (content.toLowerCase().includes('@assistant')) {
            this.showTypingIndicator();
        }
    }

    addMessage(msg, scroll = true) {
        const msgEl = document.createElement('div');
        const role = msg.role || msg.Role;
        const content = msg.content || msg.Content;
        const displayName = msg.display_name || msg.DisplayName;

        msgEl.className = `message ${role}`;

        if (role === 'user' && displayName) {
            const nameEl = document.createElement('div');
            nameEl.className = 'message-sender';
            nameEl.textContent = displayName;
            msgEl.appendChild(nameEl);
        }

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';
        contentEl.textContent = content;
        msgEl.appendChild(contentEl);

        if (msg.ID) {
            msgEl.setAttribute('data-message-id', msg.ID);
        }

        this.messagesEl.appendChild(msgEl);
        if (scroll) this.scrollToBottom();
    }

    showTypingIndicator() {
        const indicator = document.createElement('div');
        indicator.className = 'message assistant typing-indicator';
        indicator.id = 'typing-indicator';
        indicator.innerHTML = '<span></span><span></span><span></span>';
        this.messagesEl.appendChild(indicator);
        this.scrollToBottom();
    }

    removeTypingIndicator() {
        const indicator = document.getElementById('typing-indicator');
        if (indicator) indicator.remove();
    }

    scrollToBottom() {
        this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
    }

    async loadMoreHistory() {
        if (this.isLoadingHistory || !this.hasMoreHistory || !this.oldestMessageId) return;

        this.isLoadingHistory = true;
        const token = localStorage.getItem('access_token');

        try {
            const resp = await fetch(
                `/api/groups/${this.groupId}/messages/history?before=${this.oldestMessageId}&limit=50`,
                { headers: { 'Authorization': `Bearer ${token}` } }
            );

            if (!resp.ok) throw new Error('Failed to load history');

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                const scrollHeight = this.messagesEl.scrollHeight;
                const scrollTop = this.messagesEl.scrollTop;

                messages.reverse().forEach(msg => this.prependMessage(msg));
                this.oldestMessageId = messages[0].ID;

                this.messagesEl.scrollTop = this.messagesEl.scrollHeight - scrollHeight + scrollTop;
            } else {
                this.hasMoreHistory = false;
            }
        } catch (err) {
            console.error('Failed to load history:', err);
        } finally {
            this.isLoadingHistory = false;
        }
    }

    prependMessage(msg) {
        const msgEl = document.createElement('div');
        const role = msg.Role;
        msgEl.className = `message ${role}`;

        if (role === 'user' && msg.DisplayName) {
            const nameEl = document.createElement('div');
            nameEl.className = 'message-sender';
            nameEl.textContent = msg.DisplayName;
            msgEl.appendChild(nameEl);
        }

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';
        contentEl.textContent = msg.Content;
        msgEl.appendChild(contentEl);

        if (msg.ID) {
            msgEl.setAttribute('data-message-id', msg.ID);
        }

        this.messagesEl.insertBefore(msgEl, this.messagesEl.firstChild);
    }

    async leaveGroup() {
        if (!confirm('Are you sure you want to leave this group?')) return;

        const token = localStorage.getItem('access_token');
        try {
            const resp = await fetch(`/api/groups/${this.groupId}/members/${this.currentUserId}`, {
                method: 'DELETE',
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to leave group');
            window.location.href = '/groups';
        } catch (err) {
            console.error('Failed to leave group:', err);
            alert('Failed to leave group');
        }
    }

    async deleteGroup() {
        if (!confirm('Are you sure you want to delete this group? This cannot be undone.')) return;

        const token = localStorage.getItem('access_token');
        try {
            const resp = await fetch(`/api/groups/${this.groupId}`, {
                method: 'DELETE',
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to delete group');
            window.location.href = '/groups';
        } catch (err) {
            console.error('Failed to delete group:', err);
            alert('Failed to delete group');
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

document.addEventListener('DOMContentLoaded', () => new GroupChatClient(GROUP_ID));
```

**Step 3: Add page handler**

Add to `server/pages.go`:

```go
func (s *Server) handleGroupChatPage(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	s.renderTemplate(w, "group_chat", map[string]interface{}{
		"Title":   "Group Chat",
		"GroupID": groupID,
	})
}
```

Add imports for `"strconv"` and `"strings"`.

**Step 4: Add route**

Add to routes() in `server/server.go`:

```go
s.router.HandleFunc("GET /groups/{id}", s.handleGroupChatPage)
```

**Step 5: Load template**

Update loadTemplates() to include group_chat template.

**Step 6: Test manually**

Run: `go build && ./bobot`
Navigate to `/groups`, create a group, open it
Expected: Group chat page loads and works

**Step 7: Commit**

```bash
git add web/templates/group_chat.html web/static/group_chat.js server/pages.go server/server.go
git commit -m "feat(web): add group chat page"
```

---

## Task 17: Frontend - Navigation Links

**Files:**
- Modify: `web/templates/chat.html`
- Modify: `web/static/style.css`

**Step 1: Add navigation to chat header**

Update `web/templates/chat.html` header:

```html
<header class="chat-header">
    <h1>bobot</h1>
    <nav class="header-nav">
        <a href="/groups" class="nav-link">Groups</a>
        <button id="menu-btn" class="menu-btn">&#9776;</button>
    </nav>
</header>
```

**Step 2: Add CSS for navigation**

Add to `web/static/style.css`:

```css
.header-nav {
    display: flex;
    align-items: center;
    gap: 1rem;
}

.nav-link {
    color: inherit;
    text-decoration: none;
    padding: 0.5rem;
}

.nav-link:hover {
    text-decoration: underline;
}

.back-link {
    color: inherit;
    text-decoration: none;
    font-size: 1.5rem;
    padding: 0.5rem;
}

/* Group-specific styles */
.groups-container {
    max-width: 600px;
    margin: 0 auto;
    padding: 1rem;
}

.groups-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem 0;
}

.groups-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
}

.group-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem;
    background: var(--bg-secondary, #f5f5f5);
    border-radius: 8px;
    text-decoration: none;
    color: inherit;
}

.group-item:hover {
    background: var(--bg-hover, #eee);
}

.create-btn {
    font-size: 1.5rem;
    padding: 0.5rem 1rem;
    background: var(--primary, #007bff);
    color: white;
    border: none;
    border-radius: 8px;
    cursor: pointer;
}

.modal {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
}

.modal-content {
    background: white;
    padding: 2rem;
    border-radius: 8px;
    min-width: 300px;
}

.modal-actions {
    display: flex;
    gap: 1rem;
    justify-content: flex-end;
    margin-top: 1rem;
}

.message-sender {
    font-size: 0.8rem;
    font-weight: bold;
    margin-bottom: 0.25rem;
    opacity: 0.7;
}

.members-list {
    padding: 1rem 0;
}

.member {
    padding: 0.25rem 0;
}

.danger-btn {
    background: #dc3545;
    color: white;
    border: none;
    padding: 0.75rem 1rem;
    border-radius: 4px;
    cursor: pointer;
    width: 100%;
    margin-top: 0.5rem;
}

.empty, .loading, .error {
    text-align: center;
    padding: 2rem;
    color: #666;
}
```

**Step 3: Test manually**

Run: `go build && ./bobot`
Navigate between `/chat` and `/groups`
Expected: Navigation works smoothly

**Step 4: Commit**

```bash
git add web/templates/chat.html web/static/style.css
git commit -m "feat(web): add navigation between chat and groups"
```

---

## Task 18: Integration Testing

**Files:**
- Run all tests

**Step 1: Run all unit tests**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 2: Manual integration test**

1. Start server: `go build && ./bobot`
2. Log in
3. Navigate to Groups
4. Create a new group
5. Send a message
6. Send a message with @assistant
7. Verify assistant responds
8. Open another browser/incognito
9. Log in as different user
10. First user adds second user to group
11. Verify second user sees the group
12. Second user sends a message
13. Verify first user sees the message in real-time

**Step 3: Fix any issues found**

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat: complete group chat implementation"
```

---

## Summary

This implementation plan covers:

1. **Tasks 1-7**: Database schema changes (groups, group_members, messages.group_id)
2. **Tasks 8-11**: Server API endpoints (CRUD for groups, members, messages)
3. **Tasks 12-14**: WebSocket modifications (group_id in messages, group broadcast, assistant triggering)
4. **Tasks 15-17**: Frontend (groups list page, group chat page, navigation)
5. **Task 18**: Integration testing

Each task follows TDD with explicit test → implement → verify → commit cycles.
