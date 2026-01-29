// main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/server"
	"github.com/esnunes/bobot/skills"
	"github.com/esnunes/bobot/tools"
	"github.com/esnunes/bobot/tools/task"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize core database
	coreDB, err := db.NewCoreDB(filepath.Join(cfg.DataDir, "core.db"))
	if err != nil {
		log.Fatalf("Failed to initialize core database: %v", err)
	}
	defer coreDB.Close()

	// Initialize task database
	taskDB, err := task.NewTaskDB(filepath.Join(cfg.DataDir, "tool_task.db"))
	if err != nil {
		log.Fatalf("Failed to initialize task database: %v", err)
	}
	defer taskDB.Close()

	// Create initial user if configured and no users exist
	if cfg.InitUser != "" && cfg.InitPass != "" {
		count, _ := coreDB.UserCount()
		if count == 0 {
			hash, err := auth.HashPassword(cfg.InitPass)
			if err != nil {
				log.Fatalf("Failed to hash initial password: %v", err)
			}
			_, err = coreDB.CreateUser(cfg.InitUser, hash)
			if err != nil {
				log.Fatalf("Failed to create initial user: %v", err)
			}
			log.Printf("Created initial user: %s", cfg.InitUser)
		}
	}

	// Initialize JWT service
	jwtSvc := auth.NewJWTService(cfg.JWT.Secret)

	// Initialize tool registry
	registry := tools.NewRegistry()
	registry.Register(task.NewTaskTool(taskDB))

	// Load embedded skills
	loadedSkills, err := assistant.LoadSkills(skills.FS)
	if err != nil {
		log.Printf("Warning: Failed to load skills: %v", err)
	}

	// Initialize LLM provider
	llmProvider := llm.NewAnthropicClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)

	// Initialize assistant engine
	engine := assistant.NewEngine(llmProvider, registry, loadedSkills)

	// Initialize HTTP server
	srv := server.NewWithAssistant(cfg, coreDB, jwtSvc, engine)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
