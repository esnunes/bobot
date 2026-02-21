// context/adapter.go
package context

import (
	"fmt"
	"strings"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/db"
)

// CoreDBAdapter adapts CoreDB to the ContextProvider interface.
type CoreDBAdapter struct {
	db *db.CoreDB
}

// Compile-time check that CoreDBAdapter implements ContextProvider, ProfileProvider, and SkillProvider.
var _ assistant.ContextProvider = (*CoreDBAdapter)(nil)
var _ assistant.ProfileProvider = (*CoreDBAdapter)(nil)
var _ assistant.SkillProvider = (*CoreDBAdapter)(nil)

// NewCoreDBAdapter creates a new adapter.
func NewCoreDBAdapter(coreDB *db.CoreDB) *CoreDBAdapter {
	return &CoreDBAdapter{db: coreDB}
}

// GetContextMessages returns context messages for a user.
func (a *CoreDBAdapter) GetContextMessages(userID int64) ([]assistant.ContextMessage, error) {
	messages, err := a.db.GetPrivateChatContextMessages(userID)
	if err != nil {
		return nil, err
	}

	result := make([]assistant.ContextMessage, len(messages))
	for i, m := range messages {
		result[i] = assistant.ContextMessage{
			ID:         m.ID,
			Role:       m.Role,
			Content:    m.Content,
			RawContent: m.RawContent,
			CreatedAt:  m.CreatedAt,
		}
	}
	return result, nil
}

// GetTopicContextMessages returns context messages for a topic.
func (a *CoreDBAdapter) GetTopicContextMessages(topicID int64) ([]assistant.ContextMessage, error) {
	messages, err := a.db.GetTopicContextMessages(topicID)
	if err != nil {
		return nil, err
	}

	result := make([]assistant.ContextMessage, len(messages))
	for i, m := range messages {
		result[i] = assistant.ContextMessage{
			ID:         m.ID,
			Role:       m.Role,
			Content:    m.Content,
			RawContent: m.RawContent,
			CreatedAt:  m.CreatedAt,
		}
	}
	return result, nil
}

// GetUserProfile returns the profile content and last message ID for a user.
func (a *CoreDBAdapter) GetUserProfile(userID int64) (string, int64, error) {
	return a.db.GetUserProfile(userID)
}

// GetTopicMemberProfiles returns formatted profiles for all topic members.
func (a *CoreDBAdapter) GetTopicMemberProfiles(topicID int64) (string, error) {
	members, err := a.db.GetTopicMembers(topicID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	hasProfiles := false

	for _, m := range members {
		content, _, err := a.db.GetUserProfile(m.UserID)
		if err != nil || content == "" {
			continue
		}

		if !hasProfiles {
			sb.WriteString("## Topic Members\nThe following are the profiles of the members in this topic:\n")
			hasProfiles = true
		}

		name := m.DisplayName
		if name == "" {
			name = m.Username
		}
		fmt.Fprintf(&sb, "\n<member name=%q>\n%s\n</member>\n", name, content)
	}

	if !hasProfiles {
		return "", nil
	}
	return sb.String(), nil
}

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
