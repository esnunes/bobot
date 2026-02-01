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
	jwt         *auth.JWTService
	engine      *assistant.Engine
	registry    *tools.Registry
	connections *ConnectionRegistry
	router      *http.ServeMux
	templates   map[string]*template.Template
}

func New(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService) *Server {
	return NewWithAssistant(cfg, coreDB, jwt, nil, nil)
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService, engine *assistant.Engine, registry *tools.Registry) *Server {
	s := &Server{
		cfg:         cfg,
		db:          coreDB,
		jwt:         jwt,
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
	s.router.HandleFunc("POST /api/login", s.handleLogin)
	s.router.HandleFunc("POST /api/signup", s.handleSignup)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
	s.router.HandleFunc("POST /api/logout", s.handleLogout)
	s.router.HandleFunc("GET /ws/chat", s.handleChat)

	// Message routes (require auth)
	s.router.HandleFunc("GET /api/messages/recent", s.authMiddleware(s.handleRecentMessages))
	s.router.HandleFunc("GET /api/messages/history", s.authMiddleware(s.handleMessageHistory))
	s.router.HandleFunc("GET /api/messages/sync", s.authMiddleware(s.handleMessageSync))

	// Group routes (require auth)
	s.router.HandleFunc("POST /api/groups", s.authMiddleware(s.handleCreateGroup))
	s.router.HandleFunc("GET /api/groups", s.authMiddleware(s.handleListGroups))
	s.router.HandleFunc("GET /api/groups/{id}", s.authMiddleware(s.handleGetGroup))
	s.router.HandleFunc("DELETE /api/groups/{id}", s.authMiddleware(s.handleDeleteGroup))
	s.router.HandleFunc("POST /api/groups/{id}/members", s.authMiddleware(s.handleAddGroupMember))
	s.router.HandleFunc("DELETE /api/groups/{id}/members/{userId}", s.authMiddleware(s.handleRemoveGroupMember))
	s.router.HandleFunc("GET /api/groups/{id}/messages/recent", s.authMiddleware(s.handleGroupRecentMessages))
	s.router.HandleFunc("GET /api/groups/{id}/messages/history", s.authMiddleware(s.handleGroupMessageHistory))
	s.router.HandleFunc("GET /api/groups/{id}/messages/sync", s.authMiddleware(s.handleGroupMessageSync))

	// Page routes
	s.router.HandleFunc("GET /", s.handleLoginPage)
	s.router.HandleFunc("GET /signup", s.handleSignupPage)
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

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token := authHeader[7:]
		claims, err := s.jwt.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := auth.ContextWithUserData(r.Context(), auth.UserData{
			UserID: claims.UserID,
			Role:   claims.Role,
		})
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
