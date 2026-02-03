// server/auth.go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type signupRequest struct {
	Code        string `json:"code"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func validateUsername(username string) error {
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters")
	}
	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("username can only contain letters, numbers, and underscores")
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	return nil
}

func validateDisplayName(name string) error {
	if len(strings.TrimSpace(name)) < 1 {
		return fmt.Errorf("display name is required")
	}
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, err := s.db.GetUserByUsername(req.Username)
	if err == db.ErrNotFound {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if user.Blocked {
		http.Error(w, "account blocked", http.StatusForbidden)
		return
	}

	token, err := s.session.CreateToken(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.setSessionCookie(w, token)

	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/chat")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	// TODO: Remove this endpoint in Task 10 - session tokens are auto-refreshed via cookies
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement session-based logout in Task 8
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement session-based signup in Task 7
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
