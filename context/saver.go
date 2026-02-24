package context

import (
	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/db"
)

// CoreDBMessageSaver implements assistant.MessageSaver using CoreDB.
type CoreDBMessageSaver struct {
	db          *db.CoreDB
	tokensStart int
	tokensMax   int
}

var _ assistant.MessageSaver = (*CoreDBMessageSaver)(nil)

func NewCoreDBMessageSaver(coreDB *db.CoreDB, tokensStart, tokensMax int) *CoreDBMessageSaver {
	return &CoreDBMessageSaver{db: coreDB, tokensStart: tokensStart, tokensMax: tokensMax}
}

func (s *CoreDBMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	var senderID int64
	if role == "assistant" {
		senderID = db.BobotUserID
	} else {
		senderID = userID
	}

	_, err := s.db.CreateTopicMessageWithContext(
		topicID, senderID, role, content, rawContent,
		s.tokensStart, s.tokensMax,
	)
	return err
}
