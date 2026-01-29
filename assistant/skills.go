// assistant/skills.go
package assistant

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Content     string `yaml:"-"`
}

func LoadSkills(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		skill, err := loadSkillFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		skills = append(skills, *skill)
	}

	return skills, nil
}

func loadSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
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
