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
	"github.com/esnunes/bobot/web"
)

type Server struct {
	cfg         *config.Config
	db          *db.CoreDB
	jwt         *auth.JWTService
	engine      *assistant.Engine
	connections *ConnectionRegistry
	router      *http.ServeMux
	templates   map[string]*template.Template
}

func New(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService) *Server {
	return NewWithAssistant(cfg, coreDB, jwt, nil)
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService, engine *assistant.Engine) *Server {
	s := &Server{
		cfg:         cfg,
		db:          coreDB,
		jwt:         jwt,
		engine:      engine,
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

	// Page routes
	s.router.HandleFunc("GET /", s.handleLoginPage)
	s.router.HandleFunc("GET /signup", s.handleSignupPage)
	s.router.HandleFunc("GET /chat", s.handleChatPage)

	// Static files
	staticFS, _ := fs.Sub(web.FS, "static")
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

		ctx := auth.ContextWithUserID(r.Context(), claims.UserID)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
