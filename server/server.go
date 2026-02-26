// server/server.go
package server

import (
	"encoding/json"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/push"
	"github.com/esnunes/bobot/tools"
	"github.com/esnunes/bobot/tools/calendar"
	"github.com/esnunes/bobot/tools/schedule"
	"github.com/esnunes/bobot/web"
)

type Server struct {
	cfg          *config.Config
	db           *db.CoreDB
	scheduleDB   *schedule.ScheduleDB
	calendarTool *calendar.CalendarTool
	session      *auth.SessionService
	engine       *assistant.Engine
	registry     *tools.Registry
	connections  *ConnectionRegistry
	pushSender   *push.PushSender
	pipeline     *ChatPipeline
	router       *http.ServeMux
	templates    map[string]*template.Template
}

func New(cfg *config.Config, coreDB *db.CoreDB) *Server {
	return NewWithAssistant(cfg, coreDB, nil, nil, nil, nil, nil)
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, engine *assistant.Engine, registry *tools.Registry, pipeline *ChatPipeline, scheduleDB *schedule.ScheduleDB, calendarTool *calendar.CalendarTool) *Server {
	session := auth.NewSessionService(
		cfg.JWT.Secret,
		cfg.Session.Duration,
		cfg.Session.MaxAge,
		cfg.Session.RefreshThreshold,
	)

	s := &Server{
		cfg:          cfg,
		db:           coreDB,
		scheduleDB:   scheduleDB,
		calendarTool: calendarTool,
		session:      session,
		engine:       engine,
		registry:     registry,
		pipeline:     pipeline,
		router:       http.NewServeMux(),
		templates:    make(map[string]*template.Template),
	}

	// Create ConnectionRegistry (shared with pipeline if provided)
	if pipeline != nil {
		s.connections = pipeline.connections
		s.pushSender = pipeline.pushSender
	} else {
		s.connections = NewConnectionRegistry()

		// Initialize push sender if VAPID keys are configured
		if cfg.VAPID.PublicKey != "" && cfg.VAPID.PrivateKey != "" {
			ps, err := push.NewPushSender(coreDB, cfg.VAPID.PublicKey, cfg.VAPID.PrivateKey, cfg.VAPID.Subject)
			if err != nil {
				slog.Error("push: failed to initialize push sender", "error", err)
			} else {
				s.pushSender = ps
			}
		}

		// Auto-create pipeline when engine is available (backward compatibility)
		if engine != nil {
			s.pipeline = NewChatPipeline(coreDB, engine, s.connections, s.pushSender, cfg)
		}
	}

	s.loadTemplates()
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("GET /ws/chat", s.sessionMiddleware(s.handleChat))

	// Topic routes (require auth)
	s.router.HandleFunc("POST /api/topics", s.sessionMiddleware(s.handleCreateTopic))
	s.router.HandleFunc("GET /api/topics", s.sessionMiddleware(s.handleListTopics))
	s.router.HandleFunc("GET /api/topics/{id}", s.sessionMiddleware(s.handleGetTopic))
	s.router.HandleFunc("DELETE /api/topics/{id}", s.sessionMiddleware(s.handleDeleteTopic))
	s.router.HandleFunc("POST /api/topics/{id}/members", s.sessionMiddleware(s.handleAddTopicMember))
	s.router.HandleFunc("DELETE /api/topics/{id}/members/{userId}", s.sessionMiddleware(s.handleRemoveTopicMember))
	s.router.HandleFunc("POST /api/topics/{id}/mute", s.sessionMiddleware(s.handleToggleTopicMute))
	s.router.HandleFunc("DELETE /api/topics/{id}/mute", s.sessionMiddleware(s.handleToggleTopicMute))
	s.router.HandleFunc("POST /api/topics/{id}/auto-read", s.sessionMiddleware(s.handleToggleTopicAutoRead))
	s.router.HandleFunc("DELETE /api/topics/{id}/auto-read", s.sessionMiddleware(s.handleToggleTopicAutoRead))
	s.router.HandleFunc("POST /api/topics/{id}/auto-respond", s.sessionMiddleware(s.handleToggleTopicAutoRespond))
	s.router.HandleFunc("DELETE /api/topics/{id}/auto-respond", s.sessionMiddleware(s.handleToggleTopicAutoRespond))
	s.router.HandleFunc("GET /api/topics/{id}/messages/history", s.sessionMiddleware(s.handleTopicMessageHistory))
	s.router.HandleFunc("GET /api/topics/{id}/messages/sync", s.sessionMiddleware(s.handleTopicMessageSync))

	// Skill routes (require auth)
	s.router.HandleFunc("GET /skills", s.sessionMiddleware(s.handleSkillsPage))
	s.router.HandleFunc("GET /skills/new", s.sessionMiddleware(s.handleSkillFormPage))
	s.router.HandleFunc("GET /skills/{id}/edit", s.sessionMiddleware(s.handleSkillFormPage))
	s.router.HandleFunc("POST /skills", s.sessionMiddleware(s.handleCreateSkillForm))
	s.router.HandleFunc("POST /skills/{id}", s.sessionMiddleware(s.handleUpdateSkillForm))
	s.router.HandleFunc("DELETE /skills/{id}", s.sessionMiddleware(s.handleDeleteSkillForm))

	// Schedule routes (require auth)
	s.router.HandleFunc("GET /schedules", s.sessionMiddleware(s.handleSchedulesPage))
	s.router.HandleFunc("GET /schedules/new", s.sessionMiddleware(s.handleScheduleFormPage))
	s.router.HandleFunc("GET /schedules/{id}/edit", s.sessionMiddleware(s.handleScheduleFormPage))
	s.router.HandleFunc("POST /schedules", s.sessionMiddleware(s.handleCreateScheduleForm))
	s.router.HandleFunc("POST /schedules/{id}", s.sessionMiddleware(s.handleUpdateScheduleForm))
	s.router.HandleFunc("DELETE /schedules/{id}", s.sessionMiddleware(s.handleDeleteScheduleForm))

	// Quick action routes (require auth)
	s.router.HandleFunc("GET /quickactions", s.sessionMiddleware(s.handleQuickActionsPage))
	s.router.HandleFunc("GET /quickactions/new", s.sessionMiddleware(s.handleQuickActionFormPage))
	s.router.HandleFunc("GET /quickactions/{id}/edit", s.sessionMiddleware(s.handleQuickActionFormPage))
	s.router.HandleFunc("POST /quickactions", s.sessionMiddleware(s.handleCreateQuickActionForm))
	s.router.HandleFunc("POST /quickactions/{id}", s.sessionMiddleware(s.handleUpdateQuickActionForm))
	s.router.HandleFunc("DELETE /quickactions/{id}", s.sessionMiddleware(s.handleDeleteQuickActionForm))

	// Calendar OAuth routes (require auth)
	s.router.HandleFunc("GET /api/calendar/auth", s.sessionMiddleware(s.handleCalendarAuth))
	s.router.HandleFunc("GET /api/calendar/callback", s.sessionMiddleware(s.handleCalendarCallback))
	s.router.HandleFunc("GET /calendar/pick", s.sessionMiddleware(s.handleCalendarPickPage))
	s.router.HandleFunc("POST /calendar/pick", s.sessionMiddleware(s.handleCalendarPickSubmit))
	s.router.HandleFunc("DELETE /api/calendar", s.sessionMiddleware(s.handleCalendarDisconnect))

	// Admin routes (require auth + admin role)
	s.router.HandleFunc("GET /admin", s.sessionMiddleware(s.adminMiddleware(s.handleAdminPage)))
	s.router.HandleFunc("GET /admin/users/{id}", s.sessionMiddleware(s.adminMiddleware(s.handleAdminUserPage)))
	s.router.HandleFunc("GET /admin/topics/{id}/context", s.sessionMiddleware(s.adminMiddleware(s.handleAdminTopicContextPage)))

	// Push notification routes (require auth)
	s.router.HandleFunc("POST /api/push/subscribe", s.sessionMiddleware(s.handlePushSubscribe))
	s.router.HandleFunc("DELETE /api/push/subscribe", s.sessionMiddleware(s.handlePushUnsubscribe))

	// Page routes
	s.router.HandleFunc("GET /{$}", s.handleLandingPage)
	s.router.HandleFunc("GET /login", s.handleLoginPage)
	s.router.HandleFunc("POST /login", s.handleLoginPage)
	s.router.HandleFunc("GET /privacy", s.handlePrivacyPage)
	s.router.HandleFunc("GET /tos", s.handleTosPage)
	s.router.HandleFunc("POST /logout", s.handleLogout)
	s.router.HandleFunc("GET /signup", s.handleSignupPage)
	s.router.HandleFunc("POST /signup", s.handleSignupPage)
	s.router.HandleFunc("GET /chat", s.sessionMiddleware(s.handleChatPage))
	s.router.HandleFunc("GET /chats", s.sessionMiddleware(s.handleChatsPage))
	s.router.HandleFunc("GET /chats/{id}", s.sessionMiddleware(s.handleTopicChatPage))
	s.router.HandleFunc("GET /settings", s.sessionMiddleware(s.handleSettingsPage))
	s.router.HandleFunc("POST /api/user/display-name", s.sessionMiddleware(s.handleUpdateDisplayName))
	s.router.HandleFunc("POST /api/user/language", s.sessionMiddleware(s.handleUpdateLanguage))

	// Service worker (must be served at root scope)
	s.router.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(web.FS, "static/sw.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Service-Worker-Allowed", "/")
		w.Write(data)
	})

	// Static files
	staticFS, _ := fs.Sub(web.FS, "static")

	// Serve manifest.json with correct content type
	s.router.HandleFunc("GET /static/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(staticFS, "manifest.json")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Write(data)
	})

	s.router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
}

func (s *Server) sessionMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token, err := s.session.DecryptToken(cookie.Value)
		if err != nil {
			s.clearSessionCookie(w)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check absolute deadline (7 days from issue)
		if s.session.IsPastDeadline(token) {
			s.clearSessionCookie(w)
			http.Error(w, "session expired", http.StatusUnauthorized)
			return
		}

		// Check if reissue needed (expired or near expiry)
		if s.session.NeedsReissue(token) {
			// Database checks
			user, err := s.db.GetUserByID(token.UserID)
			if err != nil || user.Blocked {
				s.clearSessionCookie(w)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			hasRevocation, err := s.db.HasSessionRevocation(token.UserID, token.IssuedAt)
			if err != nil || hasRevocation {
				s.clearSessionCookie(w)
				http.Error(w, "session revoked", http.StatusUnauthorized)
				return
			}

			// Reissue token with language from DB
			newToken, err := s.session.CreateToken(token.UserID, user.Role, user.Language)
			if err == nil {
				s.setSessionCookie(w, newToken)
				token.Language = user.Language
			}
		}

		// Default language for old tokens that don't have it
		lang := token.Language
		if lang == "" {
			lang = "pt-BR"
		}

		ctx := auth.ContextWithUserData(r.Context(), auth.UserData{
			UserID:   token.UserID,
			Role:     token.Role,
			Language: lang,
		})
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userData := auth.UserDataFromContext(r.Context())
		if userData.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	secure := len(s.cfg.BaseURL) >= 5 && s.cfg.BaseURL[:5] == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.session.MaxAge().Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
