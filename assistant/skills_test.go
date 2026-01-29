// assistant/skills_test.go
package assistant

import (
	"testing"
	"testing/fstest"
)

func TestLoadSkills(t *testing.T) {
	// Create in-memory filesystem with test skill
	skillContent := `---
name: groceries
description: Manage grocery shopping lists
---
When the user wants to manage their grocery list, use the task tool.
`
	fsys := fstest.MapFS{
		"groceries.md": &fstest.MapFile{Data: []byte(skillContent)},
	}

	skills, err := LoadSkills(fsys)
	if err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	skill := skills[0]
	if skill.Name != "groceries" {
		t.Errorf("expected name groceries, got %s", skill.Name)
	}
	if skill.Description != "Manage grocery shopping lists" {
		t.Errorf("unexpected description: %s", skill.Description)
	}
	if skill.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestLoadSkills_EmptyDir(t *testing.T) {
	fsys := fstest.MapFS{}

	skills, err := LoadSkills(fsys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestLoadSkills_MultipleSkills(t *testing.T) {
	fsys := fstest.MapFS{
		"groceries.md": &fstest.MapFile{Data: []byte(`---
name: groceries
description: Manage groceries
---
Content for groceries.
`)},
		"reminders.md": &fstest.MapFile{Data: []byte(`---
name: reminders
description: Set reminders
---
Content for reminders.
`)},
		"readme.txt": &fstest.MapFile{Data: []byte("Not a skill")}, // Should be ignored
	}

	skills, err := LoadSkills(fsys)
	if err != nil {
		t.Fatalf("failed to load skills: %v", err)
	}

	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}
