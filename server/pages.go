// server/pages.go
package server

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/web"
)

type TopicView struct {
	ID          int64
	Name        string
	MemberCount int
}

type MessageView struct {
	ID          int64
	Role        string
	Content     string
	CreatedAt   string
	UserID      int64
	DisplayName string
}

type MemberView struct {
	UserID      int64
	Username    string
	DisplayName string
}

type PageData struct {
	Title         string
	Error         string
	Code          string
	TopicID       int64
	Topics        []TopicView
	Messages      []MessageView
	Members       []MemberView
	TopicName     string
	OwnerID       int64
	CurrentUserID int64
	PageDataJSON  template.JS
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

	topicsTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/topics.html")
	if err != nil {
		return err
	}
	s.templates["topics"] = topicsTmpl

	topicChatTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/topic_chat.html")
	if err != nil {
		return err
	}
	s.templates["topic_chat"] = topicChatTmpl

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

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Check for "logout everywhere" parameter
	if r.URL.Query().Get("all") == "true" {
		// Try to get user from session cookie
		if cookie, err := r.Cookie("session"); err == nil {
			if token, err := s.session.DecryptToken(cookie.Value); err == nil {
				s.db.CreateSessionRevocation(token.UserID, "logout_all")
			}
		}
	}

	s.clearSessionCookie(w)

	w.Header().Set("HX-Location", "/")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	dbMessages, _ := s.db.GetPrivateChatRecentMessages(userData.UserID, 50)

	type chatMessageJSON struct {
		ID        int64  `json:"id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}

	jsonMessages := make([]chatMessageJSON, 0, len(dbMessages))
	for _, m := range dbMessages {
		jsonMessages = append(jsonMessages, chatMessageJSON{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	jsonData, _ := json.Marshal(map[string]any{
		"messages": jsonMessages,
	})

	s.templates["chat"].Execute(w, PageData{
		Title:        "Chat",
		PageDataJSON: template.JS(jsonData),
	})
}

func (s *Server) handleTopicsPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topics, err := s.db.GetUserTopics(userData.UserID)
	if err != nil {
		http.Error(w, "failed to load topics", http.StatusInternalServerError)
		return
	}

	topicViews := make([]TopicView, 0, len(topics))
	for _, t := range topics {
		members, _ := s.db.GetTopicMembers(t.ID)
		topicViews = append(topicViews, TopicView{
			ID:          t.ID,
			Name:        t.Name,
			MemberCount: len(members),
		})
	}

	s.templates["topics"].Execute(w, PageData{Title: "Topics", Topics: topicViews})
}

func (s *Server) handleTopicChatPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	topicID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	// Check membership
	isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	dbMembers, _ := s.db.GetTopicMembers(topicID)
	members := make([]MemberView, 0, len(dbMembers))
	for _, m := range dbMembers {
		members = append(members, MemberView{
			UserID:      m.UserID,
			Username:    m.Username,
			DisplayName: m.DisplayName,
		})
	}

	type topicMessageJSON struct {
		ID          int64  `json:"id"`
		Role        string `json:"role"`
		Content     string `json:"content"`
		CreatedAt   string `json:"created_at"`
		UserID      int64  `json:"user_id"`
		DisplayName string `json:"display_name"`
	}

	dbMessages, _ := s.db.GetTopicRecentMessages(topicID, 50)
	jsonMessages := make([]topicMessageJSON, 0, len(dbMessages))
	for _, m := range dbMessages {
		jm := topicMessageJSON{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		switch m.Role {
		case "user", "command":
			if user, err := s.db.GetUserByID(m.SenderID); err == nil {
				jm.UserID = user.ID
				jm.DisplayName = user.DisplayName
			}
		case "assistant", "system":
			jm.DisplayName = "bobot"
		}
		jsonMessages = append(jsonMessages, jm)
	}

	jsonData, _ := json.Marshal(map[string]any{
		"current_user_id": userData.UserID,
		"messages":        jsonMessages,
	})

	s.templates["topic_chat"].Execute(w, PageData{
		Title:         "Topic Chat",
		TopicID:       topicID,
		TopicName:     topic.Name,
		OwnerID:       topic.OwnerID,
		CurrentUserID: userData.UserID,
		Members:       members,
		PageDataJSON:  template.JS(jsonData),
	})
}
