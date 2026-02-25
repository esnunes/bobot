package auth

import "fmt"

// CanManageTopicResource checks if a user can manage resources for a topic.
// Admins can always manage; otherwise, only the topic owner can.
func CanManageTopicResource(role string, userID, topicOwnerID int64) error {
	if role == "admin" {
		return nil
	}
	if topicOwnerID != userID {
		return fmt.Errorf("only the topic owner or admins can manage this resource")
	}
	return nil
}
