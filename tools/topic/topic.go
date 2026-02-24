package topic

import (
	"context"
	"fmt"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type TopicTool struct {
	db *db.CoreDB
}

func NewTopicTool(db *db.CoreDB) *TopicTool {
	return &TopicTool{db: db}
}

func (t *TopicTool) Name() string {
	return "topic"
}

func (t *TopicTool) Description() string {
	return "Manage topics: create, delete, leave, add/remove members, list"
}

func (t *TopicTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "delete", "leave", "add", "remove", "list"},
				"description": "The operation to perform",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Topic name",
			},
			"username": map[string]any{
				"type":        "string",
				"description": "Username for add/remove member commands",
			},
		},
		"required": []string{"command"},
	}
}

func (t *TopicTool) AdminOnly() bool {
	return false
}

func (t *TopicTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing command. Usage: /topic <command>")
	}

	result := map[string]any{"command": parts[0]}

	switch parts[0] {
	case "create":
		if len(parts) > 1 {
			result["name"] = strings.Join(parts[1:], " ")
		}
	case "delete", "leave":
		if len(parts) > 1 {
			result["name"] = strings.Join(parts[1:], " ")
		}
	case "add", "remove":
		if len(parts) > 1 {
			result["username"] = parts[1]
		}
		if len(parts) > 2 {
			result["name"] = strings.Join(parts[2:], " ")
		}
	}

	return result, nil
}

func (t *TopicTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	chatData := auth.ChatDataFromContext(ctx)

	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("missing command. Usage: /topic <command>")
	}

	name, _ := input["name"].(string)
	username, _ := input["username"].(string)

	switch command {
	case "create":
		return t.create(userData.UserID, name)
	case "delete":
		return t.deleteTopic(userData.UserID, name, chatData.TopicID)
	case "leave":
		return t.leave(userData.UserID, name, chatData.TopicID)
	case "add":
		return t.addMember(userData.UserID, username, name, chatData.TopicID)
	case "remove":
		return t.removeMember(userData.UserID, username, name, chatData.TopicID)
	case "list":
		return t.list(userData.UserID)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *TopicTool) create(userID int64, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing topic name. Usage: /topic create <name>")
	}

	topic, err := t.db.CreateTopic(name, userID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return "", fmt.Errorf("a topic with this name already exists")
		}
		return "", fmt.Errorf("failed to create topic: %w", err)
	}

	if err := t.db.AddTopicMember(topic.ID, userID); err != nil {
		return "", fmt.Errorf("failed to add creator as member: %w", err)
	}

	return fmt.Sprintf("Topic %q created.", topic.Name), nil
}

func (t *TopicTool) list(userID int64) (string, error) {
	topics, err := t.db.GetUserTopics(userID)
	if err != nil {
		return "", fmt.Errorf("failed to list topics: %w", err)
	}

	if len(topics) == 0 {
		return "No topics found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Your topics:\n")
	for _, topic := range topics {
		owner, _ := t.db.GetUserByID(topic.OwnerID)
		ownerName := "unknown"
		if owner != nil {
			if owner.ID == userID {
				ownerName = "you"
			} else {
				ownerName = owner.Username
			}
		}
		members, _ := t.db.GetTopicMembers(topic.ID)
		sb.WriteString(fmt.Sprintf("- %s (owner: %s, %d members)\n", topic.Name, ownerName, len(members)))
	}
	return sb.String(), nil
}

// resolveTopic resolves a topic from either explicit name or current topic ID.
// When both are available, explicit name takes precedence.
func (t *TopicTool) resolveTopic(name string, topicID int64) (*db.Topic, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		topic, err := t.db.GetTopicByName(name)
		if err == db.ErrNotFound {
			return nil, fmt.Errorf("topic not found: %s", name)
		}
		return topic, err
	}
	topic, err := t.db.GetTopicByID(topicID)
	if err == db.ErrNotFound {
		return nil, fmt.Errorf("topic not found")
	}
	return topic, err
}

func (t *TopicTool) deleteTopic(userID int64, name string, topicID int64) (string, error) {
	topic, err := t.resolveTopic(name, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID != userID {
		return "", fmt.Errorf("only the topic owner can delete a topic")
	}
	if err := t.db.SoftDeleteTopic(topic.ID); err != nil {
		return "", fmt.Errorf("failed to delete topic: %w", err)
	}
	return fmt.Sprintf("Topic %q deleted.", topic.Name), nil
}

func (t *TopicTool) leave(userID int64, name string, topicID int64) (string, error) {
	topic, err := t.resolveTopic(name, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID == userID {
		return "", fmt.Errorf("the owner cannot leave a topic. Use /topic delete instead")
	}
	isMember, _ := t.db.IsTopicMember(topic.ID, userID)
	if !isMember {
		return "", fmt.Errorf("you are not a member of this topic")
	}
	if err := t.db.RemoveTopicMember(topic.ID, userID); err != nil {
		return "", fmt.Errorf("failed to leave topic: %w", err)
	}
	return fmt.Sprintf("You have left the topic %q.", topic.Name), nil
}

func (t *TopicTool) addMember(userID int64, username, topicName string, topicID int64) (string, error) {
	if username == "" {
		return "", fmt.Errorf("missing username. Usage: /topic add <username> [topic-name]")
	}

	topic, err := t.resolveTopic(topicName, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID != userID {
		return "", fmt.Errorf("only the topic owner can add members")
	}

	targetUser, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if err := t.db.AddTopicMember(topic.ID, targetUser.ID); err != nil {
		// Idempotent — if already a member, that's fine
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Sprintf("%s is already a member of %q.", username, topic.Name), nil
		}
		return "", fmt.Errorf("failed to add member: %w", err)
	}

	return fmt.Sprintf("%s has been added to %q.", username, topic.Name), nil
}

func (t *TopicTool) removeMember(userID int64, username, topicName string, topicID int64) (string, error) {
	if username == "" {
		return "", fmt.Errorf("missing username. Usage: /topic remove <username> [topic-name]")
	}

	topic, err := t.resolveTopic(topicName, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID != userID {
		return "", fmt.Errorf("only the topic owner can remove members")
	}

	targetUser, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if targetUser.ID == userID {
		return "", fmt.Errorf("the owner cannot remove themselves. Use /topic delete instead")
	}

	if err := t.db.RemoveTopicMember(topic.ID, targetUser.ID); err != nil {
		return "", fmt.Errorf("failed to remove member: %w", err)
	}

	return fmt.Sprintf("%s has been removed from %q.", username, topic.Name), nil
}
