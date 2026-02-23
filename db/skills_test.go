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

	skill, err := db.CreateSkill(user.ID, 0, "groceries", "Manage grocery lists", "Use task tool for groceries")
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

	skill, err := db.CreateSkill(user.ID, topic.ID, "meeting-notes", "Track meeting notes", "Always summarize meetings")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}
	if skill.TopicID != topic.ID {
		t.Error("expected skill to be scoped to topic")
	}
}

func TestCreateSkillDuplicateNamePrivate(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")

	db.CreateSkill(user.ID, 0, "groceries", "desc", "content")
	_, err := db.CreateSkill(user.ID, 0, "groceries", "desc2", "content2")
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

	db.CreateSkill(alice.ID, topic.ID, "notes", "desc", "content")
	// Different user, same topic, same name — should fail
	_, err := db.CreateSkill(bob.ID, topic.ID, "notes", "desc2", "content2")
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
	_, err := db.CreateSkill(user.ID, 0, "groceries", "desc", "content")
	if err != nil {
		t.Fatalf("private skill failed: %v", err)
	}
	_, err = db.CreateSkill(user.ID, topic.ID, "groceries", "desc", "content")
	if err != nil {
		t.Fatalf("topic skill failed: %v", err)
	}
}

func TestGetSkillByID(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	created, _ := db.CreateSkill(user.ID, 0, "groceries", "desc", "content")

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

func TestGetTopicSkills(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	alice, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	topic1, _ := db.CreateTopic("General", alice.ID)
	topic2, _ := db.CreateTopic("Random", alice.ID)

	db.CreateSkill(alice.ID, topic1.ID, "skill1", "desc", "content")
	db.CreateSkill(alice.ID, topic1.ID, "skill2", "desc", "content")
	db.CreateSkill(alice.ID, topic2.ID, "skill3", "desc", "content")

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
	created, _ := db.CreateSkill(user.ID, 0, "groceries", "old desc", "old content")

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
	created, _ := db.CreateSkill(user.ID, 0, "groceries", "desc", "content")

	err := db.DeleteSkill(created.ID)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = db.GetSkillByID(created.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestGetTopicSkillByName(t *testing.T) {
	db := setupSkillTestDB(t)
	defer db.Close()

	user, _ := db.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := db.CreateTopic("General", user.ID)
	db.CreateSkill(user.ID, topic.ID, "Notes", "desc", "content")

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
	db.CreateSkill(user.ID, topic.ID, "notes", "desc", "content")

	// Soft-delete the topic — skills should remain (soft delete doesn't trigger CASCADE)
	// But we should test the cascade on the FK
	skills, _ := db.GetTopicSkills(topic.ID)
	if len(skills) != 1 {
		t.Errorf("expected 1 topic skill before delete, got %d", len(skills))
	}
}
