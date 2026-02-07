package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
)

const profileSystemPrompt = `You are a profile extraction assistant. Given a user's current profile (which may be empty) and their recent messages, produce an updated profile summary.

Extract and maintain:
- Personal details: name, location, timezone, language, job/role, company
- Preferences: communication style, response format preferences, interests, hobbies, topics they care about

Rules:
- Write in third person, concise natural language
- Preserve existing information unless explicitly contradicted
- Only add information the user has clearly stated or implied
- If the current profile is empty, create one from scratch
- Do not invent or assume information
- Output ONLY the updated profile text, nothing else`

func runUpdateProfiles(cfg *config.Config, coreDB *db.CoreDB) {
	llmProvider := llm.NewAnthropicClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)

	users, err := coreDB.ListActiveUsers()
	if err != nil {
		log.Fatalf("Failed to list users: %v", err)
	}

	log.Printf("Processing %d users...", len(users))

	for _, user := range users {
		profile, lastMsgID, err := coreDB.GetUserProfile(user.ID)
		if err != nil {
			log.Printf("Error getting profile for user %s: %v", user.Username, err)
			continue
		}

		messages, err := coreDB.GetUserMessagesSince(user.ID, lastMsgID)
		if err != nil {
			log.Printf("Error getting messages for user %s: %v", user.Username, err)
			continue
		}

		if len(messages) == 0 {
			log.Printf("Skipping %s: no new messages", user.Username)
			continue
		}

		log.Printf("Processing %s: %d new messages since message ID %d", user.Username, len(messages), lastMsgID)

		// Build user message
		profileText := profile
		if profileText == "" {
			profileText = "No profile yet."
		}

		var msgLines []string
		for _, m := range messages {
			msgLines = append(msgLines, m.Content)
		}

		userMessage := fmt.Sprintf("Current profile:\n<profile>\n%s\n</profile>\n\nNew messages:\n<messages>\n%s\n</messages>", profileText, strings.Join(msgLines, "\n"))

		resp, err := llmProvider.Chat(context.Background(), &llm.ChatRequest{
			SystemPrompt: profileSystemPrompt,
			Messages: []llm.Message{
				{Role: "user", Content: userMessage},
			},
		})
		if err != nil {
			log.Printf("LLM error for user %s: %v", user.Username, err)
			continue
		}

		newLastMsgID := messages[len(messages)-1].ID
		err = coreDB.UpsertUserProfile(user.ID, resp.Content, newLastMsgID)
		if err != nil {
			log.Printf("Error saving profile for user %s: %v", user.Username, err)
			continue
		}

		log.Printf("Updated profile for %s (last_message_id: %d)", user.Username, newLastMsgID)
	}

	log.Println("Done.")
}
