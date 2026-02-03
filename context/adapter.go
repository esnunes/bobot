// context/adapter.go
package context

import (
	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/db"
)

// CoreDBAdapter adapts CoreDB to the ContextProvider interface.
type CoreDBAdapter struct {
	db *db.CoreDB
}

// Compile-time check that CoreDBAdapter implements ContextProvider.
var _ assistant.ContextProvider = (*CoreDBAdapter)(nil)

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
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return result, nil
}
