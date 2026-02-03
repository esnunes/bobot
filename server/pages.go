// server/pages.go
package server

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/web"
)

type PageData struct {
	Title   string
	Error   string
	Code    string
	GroupID int64
}

func (s *Server) loadTemplates() error {
	loginTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/login.html")
	if err != nil {
		return err
	}
	s.templates["login"] = loginTmpl

	signupTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/signup.html")
	if err != nil {
		return err
	}
	s.templates["signup"] = signupTmpl

	chatTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/chat.html")
	if err != nil {
		return err
	}
	s.templates["chat"] = chatTmpl

	groupsTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/groups.html")
	if err != nil {
		return err
	}
	s.templates["groups"] = groupsTmpl

	groupChatTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/group_chat.html")
	if err != nil {
		return err
	}
	s.templates["group_chat"] = groupChatTmpl

	authTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/authenticated.html")
	if err != nil {
		return err
	}
	s.templates["authenticated"] = authTmpl

	return nil
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// Check if already authenticated
	if cookie, err := r.Cookie("session"); err == nil {
		if _, err := s.session.DecryptToken(cookie.Value); err == nil {
			s.templates["authenticated"].Execute(w, PageData{Title: "Loading"})
			return
		}
	}

	// GET request - show login form
	if r.Method == http.MethodGet {
		s.templates["login"].Execute(w, PageData{Title: "Login"})
		return
	}

	// POST request - handle login
	if err := r.ParseForm(); err != nil {
		s.templates["login"].Execute(w, PageData{Title: "Login", Error: "Invalid request"})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.db.GetUserByUsername(username)
	if err != nil {
		s.templates["login"].Execute(w, PageData{Title: "Login", Error: "Invalid credentials"})
		return
	}

	if !auth.CheckPassword(password, user.PasswordHash) {
		s.templates["login"].Execute(w, PageData{Title: "Login", Error: "Invalid credentials"})
		return
	}

	if user.Blocked {
		s.templates["login"].Execute(w, PageData{Title: "Login", Error: "Account blocked"})
		return
	}

	token, err := s.session.CreateToken(user.ID, user.Role)
	if err != nil {
		s.templates["login"].Execute(w, PageData{Title: "Login", Error: "Internal error"})
		return
	}

	s.setSessionCookie(w, token)
	s.templates["authenticated"].Execute(w, PageData{Title: "Loading"})
}

func (s *Server) handleSignupPage(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: "Invite code required",
		})
		return
	}

	// Validate code exists and is valid
	invite, err := s.db.GetInviteByCode(code)
	if err != nil || invite.UsedBy != nil || invite.Revoked {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: "Invalid or expired invite",
		})
		return
	}

	s.templates["signup"].Execute(w, PageData{
		Title: "Sign Up",
		Code:  code,
	})
}

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	s.templates["chat"].Execute(w, PageData{Title: "Chat"})
}

func (s *Server) handleGroupsPage(w http.ResponseWriter, r *http.Request) {
	s.templates["groups"].Execute(w, PageData{Title: "Groups"})
}

func (s *Server) handleGroupChatPage(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	s.templates["group_chat"].Execute(w, PageData{
		Title:   "Group Chat",
		GroupID: groupID,
	})
}
