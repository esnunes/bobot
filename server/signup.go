package server

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleSignupPage(w http.ResponseWriter, r *http.Request) {
	// GET request - show signup form
	if r.Method == http.MethodGet {
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
		return
	}

	// POST request - handle signup
	if err := r.ParseForm(); err != nil {
		s.templates["signup"].Execute(w, PageData{Title: "Sign Up", Error: "Invalid request"})
		return
	}

	code := r.FormValue("code")
	username := r.FormValue("username")
	displayName := r.FormValue("display_name")
	password := r.FormValue("password")

	// Validate invite code
	invite, err := s.db.GetInviteByCode(code)
	if err != nil || invite.UsedBy != nil || invite.Revoked {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: "Invalid or expired invite",
			Code:  code,
		})
		return
	}

	// Validate username
	if err := validateUsername(username); err != nil {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: err.Error(),
			Code:  code,
		})
		return
	}

	// Validate display name
	if err := validateDisplayName(displayName); err != nil {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: err.Error(),
			Code:  code,
		})
		return
	}

	// Validate password
	if err := validatePassword(password); err != nil {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: err.Error(),
			Code:  code,
		})
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: "Internal error",
			Code:  code,
		})
		return
	}

	// Create user
	user, err := s.db.CreateUserFull(username, passwordHash, displayName, "user")
	if err != nil {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: "Username already taken",
			Code:  code,
		})
		return
	}

	// Send welcome message from Bobot
	if _, err := s.db.CreateMessage(db.BobotUserID, user.ID, "assistant", db.WelcomeMessage, db.WelcomeMessage); err != nil {
		log.Printf("failed to create welcome message for user %d: %v", user.ID, err)
	}

	// Mark invite as used
	if err := s.db.UseInvite(code, user.ID); err != nil {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: "Internal error",
			Code:  code,
		})
		return
	}

	// Create session token
	token, err := s.session.CreateToken(user.ID, user.Role)
	if err != nil {
		s.templates["signup"].Execute(w, PageData{
			Title: "Sign Up",
			Error: "Internal error",
			Code:  code,
		})
		return
	}

	s.setSessionCookie(w, token)
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusNoContent)
}

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

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
