// assistant/skills.go
package assistant

import (
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Content     string `yaml:"-"`
}

// LoadSkills loads skills from an embedded filesystem.
// The fsys should contain .md files with YAML frontmatter.
func LoadSkills(fsys fs.FS) ([]Skill, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		skill, err := loadSkillFile(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		skills = append(skills, *skill)
	}

	return skills, nil
}

func loadSkillFile(fsys fs.FS, name string) (*Skill, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// Parse YAML frontmatter
	if !strings.HasPrefix(content, "---") {
		return &Skill{Content: content}, nil
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) != 2 {
		return &Skill{Content: content}, nil
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(parts[0]), &skill); err != nil {
		return nil, err
	}

	skill.Content = strings.TrimSpace(parts[1])
	return &skill, nil
}
