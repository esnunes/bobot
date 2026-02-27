// main.go
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/config"
	bobotcontext "github.com/esnunes/bobot/context"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/push"
	"github.com/esnunes/bobot/scheduler"
	"github.com/esnunes/bobot/server"
	"github.com/esnunes/bobot/skills"
	"github.com/esnunes/bobot/tools"
	"github.com/esnunes/bobot/tools/schedule"
	"github.com/esnunes/bobot/tools/skill"
	"github.com/esnunes/bobot/tools/task"
	"github.com/esnunes/bobot/tools/quickaction"
	"github.com/esnunes/bobot/tools/calendar"
	"github.com/esnunes/bobot/tools/spotify"
	"github.com/esnunes/bobot/tools/thinq"
	"github.com/esnunes/bobot/tools/topic"
	"github.com/esnunes/bobot/tools/user"
	"github.com/esnunes/bobot/tools/websearch"
)

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	// Configure structured logging
	logLevel := parseLogLevel(os.Getenv("LOG_LEVEL"))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

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

	// Initialize schedule database
	scheduleDB, err := schedule.NewScheduleDB(filepath.Join(cfg.DataDir, "tool_schedule.db"))
	if err != nil {
		log.Fatalf("Failed to initialize schedule database: %v", err)
	}
	defer scheduleDB.Close()

	// Migrate orphaned schedules (reminders/cron jobs with no topic) to bobot topics
	if err := scheduleDB.MigrateOrphanedToTopics(func(userID int64) int64 {
		topic, err := coreDB.GetUserBobotTopic(userID)
		if err != nil || topic == nil {
			return 0
		}
		return topic.ID
	}); err != nil {
		log.Fatalf("Failed to migrate orphaned schedules: %v", err)
	}

	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "create-admin":
			runCreateAdmin(coreDB)
			return
		case "update-profiles":
			runUpdateProfiles(cfg, coreDB)
			return
		case "generate-vapid-keys":
			runGenerateVAPIDKeys()
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
	registry.Register(skill.NewSkillTool(coreDB))
	registry.Register(quickaction.NewQuickActionTool(coreDB))
	registry.Register(schedule.NewRemindTool(scheduleDB))
	registry.Register(schedule.NewCronTool(scheduleDB))

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

	// Initialize Google Calendar tool (optional, only if configured)
	var calendarTool *calendar.CalendarTool
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		calendarDB, err := calendar.NewCalendarDB(filepath.Join(cfg.DataDir, "tool_calendar.db"))
		if err != nil {
			log.Fatalf("Failed to initialize calendar database: %v", err)
		}
		defer calendarDB.Close()
		calendarTool = calendar.NewCalendarTool(calendarDB, cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.BaseURL)
		registry.Register(calendarTool)
	}

	// Initialize Spotify tool (optional, only if configured)
	var spotifyTool *spotify.SpotifyTool
	if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
		spotifyDB, err := spotify.NewSpotifyDB(filepath.Join(cfg.DataDir, "tool_spotify.db"))
		if err != nil {
			log.Fatalf("Failed to initialize spotify database: %v", err)
		}
		defer spotifyDB.Close()
		spotifyTool = spotify.NewSpotifyTool(spotifyDB, cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.BaseURL)
		registry.Register(spotifyTool)
	}

	// Initialize web search tool (optional, only if configured)
	if cfg.BraveSearchAPIKey != "" {
		registry.Register(websearch.NewTool(cfg.BraveSearchAPIKey))
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
	messageSaver := bobotcontext.NewCoreDBMessageSaver(coreDB, cfg.Context.TokensStart, cfg.Context.TokensMax)

	// Initialize assistant engine with context
	engine := assistant.NewEngine(llmProvider, registry, loadedSkills, contextAdapter, contextAdapter)
	engine.SetMessageSaver(messageSaver)
	engine.SetSkillProvider(contextAdapter)

	// Create shared components for pipeline
	connections := server.NewConnectionRegistry()
	var pushSender *push.PushSender
	if cfg.VAPID.PublicKey != "" && cfg.VAPID.PrivateKey != "" {
		ps, err := push.NewPushSender(coreDB, cfg.VAPID.PublicKey, cfg.VAPID.PrivateKey, cfg.VAPID.Subject)
		if err != nil {
			slog.Error("push: failed to initialize push sender", "error", err)
		} else {
			pushSender = ps
		}
	}

	// Create ChatPipeline (shared by server and scheduler)
	pipeline := server.NewChatPipeline(coreDB, engine, connections, pushSender, cfg)

	// Initialize HTTP server
	srv := server.NewWithAssistant(cfg, coreDB, engine, registry, pipeline, scheduleDB, calendarTool, spotifyTool)

	// Create scheduler
	sched := scheduler.New(scheduleDB, coreDB, pipeline, cfg.Schedule.Timeout)

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start scheduler in background
	go sched.Start(ctx)

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	go func() {
		log.Printf("Starting server on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sig := <-sigCh
	slog.Info("received shutdown signal", "signal", sig)

	// 1. Cancel scheduler context (no new executions start)
	cancel()

	// 2. Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	// 3. Wait for scheduler to finish in-flight work
	select {
	case <-sched.Done():
		slog.Info("scheduler stopped")
	case <-time.After(cfg.Schedule.Timeout + 5*time.Second):
		slog.Warn("scheduler did not stop in time")
	}

	slog.Info("shutdown complete")
}
