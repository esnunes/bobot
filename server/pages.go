// server/pages.go
package server

import (
	"html/template"
	"net/http"

	"github.com/esnunes/bobot/web"
)

type PageData struct {
	Title string
}

func (s *Server) loadTemplates() error {
	loginTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/login.html")
	if err != nil {
		return err
	}
	s.templates["login"] = loginTmpl

	chatTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/chat.html")
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
