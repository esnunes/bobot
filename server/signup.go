package server

import (
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/i18n"
)

func (s *Server) handleSignupPage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.MatchLanguage(r.Header.Get("Accept-Language"))

	// GET request - show signup form
	if r.Method == http.MethodGet {
		code := r.URL.Query().Get("code")
		if code == "" {
			s.render(w, r, "signup", PageData{
				Title: "Sign Up",
				Error: i18n.T(lang, "signup.error.invite_required"),
			})
			return
		}

		// Validate code exists and is valid
		invite, err := s.db.GetInviteByCode(code)
		if err != nil || invite.UsedBy != nil || invite.Revoked {
			s.render(w, r, "signup", PageData{
				Title: "Sign Up",
				Error: i18n.T(lang, "signup.error.invite_invalid"),
			})
			return
		}

		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Code:  code,
		})
		return
	}

	// POST request - handle signup
	if err := r.ParseForm(); err != nil {
		s.render(w, r, "signup", PageData{Title: "Sign Up", Error: i18n.T(lang, "signup.error.invalid_request")})
		return
	}

	code := r.FormValue("code")
	username := r.FormValue("username")
	displayName := r.FormValue("display_name")
	password := r.FormValue("password")

	// Validate invite code
	invite, err := s.db.GetInviteByCode(code)
	if err != nil || invite.UsedBy != nil || invite.Revoked {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, "signup.error.invite_invalid"),
			Code:  code,
		})
		return
	}

	// Validate username
	if errKey := validateUsername(username); errKey != "" {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, errKey),
			Code:  code,
		})
		return
	}

	// Validate display name
	if errKey := validateDisplayName(displayName); errKey != "" {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, errKey),
			Code:  code,
		})
		return
	}

	// Validate password
	if errKey := validatePassword(password); errKey != "" {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, errKey),
			Code:  code,
		})
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, "signup.error.internal"),
			Code:  code,
		})
		return
	}

	// Create user
	user, err := s.db.CreateUserFull(username, passwordHash, displayName, "user")
	if err != nil {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, "signup.error.username_taken"),
			Code:  code,
		})
		return
	}

	// Create bobot topic for the new user
	bobotTopic, err := s.db.CreateBobotTopic(user.ID)
	if err != nil {
		log.Printf("failed to create bobot topic for user %d: %v", user.ID, err)
	}

	// Send welcome message in the bobot topic
	if bobotTopic != nil {
		if _, err := s.db.CreateTopicMessageWithContext(
			bobotTopic.ID, db.BobotUserID, "assistant", db.WelcomeMessage, db.WelcomeMessage,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		); err != nil {
			log.Printf("failed to create welcome message for user %d: %v", user.ID, err)
		}
	}

	// Mark invite as used
	if err := s.db.UseInvite(code, user.ID); err != nil {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, "signup.error.internal"),
			Code:  code,
		})
		return
	}

	// Create session token (new user gets default language from DB)
	token, err := s.session.CreateToken(user.ID, user.Role, user.Language)
	if err != nil {
		s.render(w, r, "signup", PageData{
			Title: "Sign Up",
			Error: i18n.T(lang, "signup.error.internal"),
			Code:  code,
		})
		return
	}

	s.setSessionCookie(w, token)
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusNoContent)
}

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// validateUsername returns an i18n key if validation fails, or empty string if valid.
func validateUsername(username string) string {
	if len(username) < 3 {
		return "signup.error.username_min"
	}
	if !usernameRegex.MatchString(username) {
		return "signup.error.username_chars"
	}
	return ""
}

// validatePassword returns an i18n key if validation fails, or empty string if valid.
func validatePassword(password string) string {
	if len(password) < 8 {
		return "signup.error.password_min"
	}
	return ""
}

// validateDisplayName returns an i18n key if validation fails, or empty string if valid.
func validateDisplayName(name string) string {
	if len(strings.TrimSpace(name)) < 1 {
		return "signup.error.display_name_required"
	}
	return ""
}
