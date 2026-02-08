// main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/config"
	bobotcontext "github.com/esnunes/bobot/context"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/server"
	"github.com/esnunes/bobot/skills"
	"github.com/esnunes/bobot/tools"
	"github.com/esnunes/bobot/tools/task"
	"github.com/esnunes/bobot/tools/thinq"
	"github.com/esnunes/bobot/tools/topic"
	"github.com/esnunes/bobot/tools/user"
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

	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "create-admin":
			runCreateAdmin(coreDB)
			return
		case "update-profiles":
			runUpdateProfiles(cfg, coreDB)
			return
		default:
			log.Fatalf("Unknown command: %s", os.Args[1])
		}
	}

	// Initialize tool registry
	registry := tools.NewRegistry()
	registry.Register(task.NewTaskTool(taskDB))
	registry.Register(user.NewUserTool(coreDB, cfg.BaseURL))
	registry.Register(topic.NewTopicTool(coreDB))

	// Initialize ThinQ tool (optional, only if configured)
	if thinqToken := os.Getenv("THINQ_TOKEN"); thinqToken != "" {
		thinqClient := thinq.NewClient(thinqToken, os.Getenv("THINQ_COUNTRY"), os.Getenv("THINQ_CLIENT_ID"))
		thinqDB, err := thinq.NewThinqDB(filepath.Join(cfg.DataDir, "tool_thinq.db"))
		if err != nil {
			log.Fatalf("Failed to initialize thinq database: %v", err)
		}
		defer thinqDB.Close()
		registry.Register(thinq.NewThinqTool(thinqClient, thinqDB))
	}

	// Load embedded skills
	loadedSkills, err := assistant.LoadSkills(skills.FS)
	if err != nil {
		log.Printf("Warning: Failed to load skills: %v", err)
	}

	// Initialize LLM provider
	llmProvider := llm.NewAnthropicClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)

	// Create context adapter
	contextAdapter := bobotcontext.NewCoreDBAdapter(coreDB)

	// Initialize assistant engine with context
	engine := assistant.NewEngine(llmProvider, registry, loadedSkills, contextAdapter, contextAdapter)

	// Initialize HTTP server
	srv := server.NewWithAssistant(cfg, coreDB, engine, registry)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
