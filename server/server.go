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
	cfg       *config.Config
	db        *db.CoreDB
	jwt       *auth.JWTService
	engine    *assistant.Engine
	router    *http.ServeMux
	templates map[string]*template.Template
}

func New(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService) *Server {
	return NewWithAssistant(cfg, coreDB, jwt, nil)
}

func NewWithAssistant(cfg *config.Config, coreDB *db.CoreDB, jwt *auth.JWTService, engine *assistant.Engine) *Server {
	s := &Server{
		cfg:       cfg,
		db:        coreDB,
		jwt:       jwt,
		engine:    engine,
		router:    http.NewServeMux(),
		templates: make(map[string]*template.Template),
	}

	s.loadTemplates()
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/login", s.handleLogin)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
	s.router.HandleFunc("POST /api/logout", s.handleLogout)
	s.router.HandleFunc("GET /ws/chat", s.handleChat)

	// Page routes
	s.router.HandleFunc("GET /", s.handleLoginPage)
	s.router.HandleFunc("GET /chat", s.handleChatPage)

	// Static files
	staticFS, _ := fs.Sub(web.FS, "static")
	s.router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
