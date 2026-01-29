// server/chat.go
package server

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/esnunes/bobot/auth"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type chatMessage struct {
	Content string `json:"content"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Get token from query param
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := s.jwt.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Create context with user ID
	ctx := auth.ContextWithUserID(r.Context(), claims.UserID)

	// Handle messages
	for {
		var msg chatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		// Save user message
		s.db.CreateMessage(claims.UserID, "user", msg.Content)

		// Get assistant response
		response, err := s.engine.Chat(ctx, msg.Content)
		if err != nil {
			log.Printf("assistant error: %v", err)
			response = "Sorry, I encountered an error. Please try again."
		}

		// Save assistant message
		s.db.CreateMessage(claims.UserID, "assistant", response)

		// Send response
		if err := conn.WriteJSON(chatMessage{Content: response}); err != nil {
			log.Printf("websocket write error: %v", err)
			break
		}
	}
}
