// server/auth.go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type signupRequest struct {
	Code        string `json:"code"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
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

	// Check if user is blocked
	if user.Blocked {
		http.Error(w, "account blocked", http.StatusForbidden)
		return
	}

	accessToken, err := s.jwt.GenerateAccessTokenWithRole(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	refreshToken := s.jwt.GenerateRefreshToken()
	expiresAt := time.Now().Add(s.jwt.RefreshTTL())

	_, err = s.db.CreateRefreshToken(user.ID, refreshToken, expiresAt)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	token, err := s.db.GetRefreshToken(req.RefreshToken)
	if err == db.ErrNotFound {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if time.Now().After(token.ExpiresAt) {
		s.db.DeleteRefreshToken(req.RefreshToken)
		http.Error(w, "token expired", http.StatusUnauthorized)
		return
	}

	// Check if user is blocked
	user, err := s.db.GetUserByID(token.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user.Blocked {
		s.db.DeleteRefreshToken(req.RefreshToken)
		http.Error(w, "account blocked", http.StatusForbidden)
		return
	}

	accessToken, err := s.jwt.GenerateAccessTokenWithRole(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	s.db.DeleteRefreshToken(req.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Validate invite code
	invite, err := s.db.GetInviteByCode(req.Code)
	if err == db.ErrNotFound {
		http.Error(w, "invalid invite code", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if invite.UsedBy != nil || invite.Revoked {
		http.Error(w, "invite code already used or revoked", http.StatusBadRequest)
		return
	}

	// Validate inputs
	req.Username = strings.ToLower(req.Username)
	if err := validateUsername(req.Username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validatePassword(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateDisplayName(req.DisplayName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check username uniqueness
	_, err = s.db.GetUserByUsername(req.Username)
	if err == nil {
		http.Error(w, "username already taken", http.StatusBadRequest)
		return
	}
	if err != db.ErrNotFound {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Create user
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	user, err := s.db.CreateUserFull(req.Username, hash, req.DisplayName, "user")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Mark invite as used
	if err := s.db.UseInvite(req.Code, user.ID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Generate tokens
	accessToken, err := s.jwt.GenerateAccessTokenWithRole(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	refreshToken := s.jwt.GenerateRefreshToken()
	expiresAt := time.Now().Add(s.jwt.RefreshTTL())

	_, err = s.db.CreateRefreshToken(user.ID, refreshToken, expiresAt)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}
