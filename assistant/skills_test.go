// assistant/skills_test.go
package assistant

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkills(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	os.MkdirAll(skillsDir, 0755)

	// Create test skill
	skillContent := `---
name: groceries
description: Manage grocery shopping lists
---
When the user wants to manage their grocery list, use the task tool.
`
	os.WriteFile(filepath.Join(skillsDir, "groceries.md"), []byte(skillContent), 0644)

	skills, err := LoadSkills(skillsDir)
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
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	os.MkdirAll(skillsDir, 0755)

	skills, err := LoadSkills(skillsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}
