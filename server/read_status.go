// server/read_status.go
package server

import (
	"encoding/json"

	"github.com/esnunes/bobot/db"
)

// markChatReadImplicit marks a chat as read using the latest message ID.
// Used when a user opens a chat page or sends a message.
func (s *Server) markChatReadImplicit(userID int64, topicID int64) {
	var latestID int64
	var err error
	if topicID == db.PrivateChatTopicID {
		latestID, err = s.db.GetLatestPrivateMessageID(userID)
	} else {
		latestID, err = s.db.GetLatestTopicMessageID(topicID)
	}
	if err != nil || latestID == 0 {
		return
	}
	s.db.MarkChatRead(userID, topicID, latestID)
	s.broadcastReadEvent(userID, topicID)
}

func (s *Server) broadcastReadEvent(userID int64, topicID int64) {
	readEvent, _ := json.Marshal(map[string]any{
		"type":     "read",
		"topic_id": topicID,
	})
	s.connections.Broadcast(userID, readEvent)
}

// autoMarkReadForTopic marks the topic as read for all members with auto-read enabled.
func (s *Server) autoMarkReadForTopic(topicID int64) {
	members, err := s.db.GetTopicMembers(topicID)
	if err != nil {
		return
	}
	for _, member := range members {
		if member.AutoRead {
			s.markChatReadImplicit(member.UserID, topicID)
		}
	}
}
