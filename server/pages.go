// server/pages.go
package server

import (
	"html/template"
	"net/http"
	"path/filepath"
)

type PageData struct {
	Title string
}

func (s *Server) loadTemplates() error {
	layout := filepath.Join(s.cfg.WebDir, "templates", "layout.html")

	loginTmpl, err := template.ParseFiles(layout, filepath.Join(s.cfg.WebDir, "templates", "login.html"))
	if err != nil {
		return err
	}
	s.templates["login"] = loginTmpl

	chatTmpl, err := template.ParseFiles(layout, filepath.Join(s.cfg.WebDir, "templates", "chat.html"))
	if err != nil {
		return err
	}
	s.templates["chat"] = chatTmpl

	return nil
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.templates["login"].Execute(w, PageData{Title: "Login"})
}

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	s.templates["chat"].Execute(w, PageData{Title: "Chat"})
}
