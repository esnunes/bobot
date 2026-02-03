// server/server.go
package server

import (
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools"
	"github.com/esnunes/bobot/web"
)

type Server struct {
	cfg         *config.Config
	db          *db.CoreDB
	session     *auth.SessionService
	engine      *assistant.Engine
	registry    *tools.Registry
	connections *ConnectionRegistry
	router      *http.ServeMux
	templates   map[string]*template.Template
}

func New(cfg *config.Config, coreDB *db.CoreDB) *Server {
	return NewWithAssistant(cfg, coreDB, nil, nil)
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, engine *assistant.Engine, registry *tools.Registry) *Server {
	session := auth.NewSessionService(
		cfg.JWT.Secret,
		cfg.Session.Duration,
		cfg.Session.MaxAge,
		cfg.Session.RefreshThreshold,
	)

	s := &Server{
		cfg:         cfg,
		db:          coreDB,
		session:     session,
		engine:      engine,
		registry:    registry,
		connections: NewConnectionRegistry(),
		router:      http.NewServeMux(),
		templates:   make(map[string]*template.Template),
	}

	s.loadTemplates()
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/logout", s.handleLogout)
	s.router.HandleFunc("GET /ws/chat", s.handleChat)

	// Message routes (require auth)
	s.router.HandleFunc("GET /api/messages/recent", s.sessionMiddleware(s.handleRecentMessages))
	s.router.HandleFunc("GET /api/messages/history", s.sessionMiddleware(s.handleMessageHistory))
	s.router.HandleFunc("GET /api/messages/sync", s.sessionMiddleware(s.handleMessageSync))

	// Group routes (require auth)
	s.router.HandleFunc("POST /api/groups", s.sessionMiddleware(s.handleCreateGroup))
	s.router.HandleFunc("GET /api/groups", s.sessionMiddleware(s.handleListGroups))
	s.router.HandleFunc("GET /api/groups/{id}", s.sessionMiddleware(s.handleGetGroup))
	s.router.HandleFunc("DELETE /api/groups/{id}", s.sessionMiddleware(s.handleDeleteGroup))
	s.router.HandleFunc("POST /api/groups/{id}/members", s.sessionMiddleware(s.handleAddGroupMember))
	s.router.HandleFunc("DELETE /api/groups/{id}/members/{userId}", s.sessionMiddleware(s.handleRemoveGroupMember))
	s.router.HandleFunc("GET /api/groups/{id}/messages/recent", s.sessionMiddleware(s.handleGroupRecentMessages))
	s.router.HandleFunc("GET /api/groups/{id}/messages/history", s.sessionMiddleware(s.handleGroupMessageHistory))
	s.router.HandleFunc("GET /api/groups/{id}/messages/sync", s.sessionMiddleware(s.handleGroupMessageSync))

	// Page routes
	s.router.HandleFunc("GET /{$}", s.handleLoginPage)
	s.router.HandleFunc("POST /{$}", s.handleLoginPage)
	s.router.HandleFunc("GET /signup", s.handleSignupPage)
	s.router.HandleFunc("POST /signup", s.handleSignupPage)
	s.router.HandleFunc("GET /chat", s.handleChatPage)
	s.router.HandleFunc("GET /groups", s.handleGroupsPage)
	s.router.HandleFunc("GET /groups/{id}", s.handleGroupChatPage)

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

			// Reissue token
			newToken, err := s.session.CreateToken(token.UserID, token.Role)
			if err == nil {
				s.setSessionCookie(w, newToken)
			}
		}

		ctx := auth.ContextWithUserData(r.Context(), auth.UserData{
			UserID: token.UserID,
			Role:   token.Role,
		})
		next(w, r.WithContext(ctx))
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
