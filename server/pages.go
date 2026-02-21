// server/pages.go
package server

import (
	"encoding/json"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/web"
)

var navigatePathRe = regexp.MustCompile(`^/chats/\d+$`)

type TopicView struct {
	ID          int64
	Name        string
	MemberCount int
	HasUnread   bool
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

type SkillView struct {
	ID          int64
	Name        string
	Description string
	Content     string
}

type ScheduleView struct {
	ID            int64
	Name          string
	Prompt        string
	PromptPreview string
	CronExpr      string
	Enabled       bool
	NextRunAt     string
}

type AdminUserView struct {
	ID          int64
	DisplayName string
	Username    string
	Role        string
	Blocked     bool
	CreatedAt   string
}

type AdminTopicView struct {
	ID          int64
	Name        string
	OwnerName   string
	MemberCount int
	CreatedAt   string
}

type ToolBlockView struct {
	Type      string // "tool_use" or "tool_result"
	ToolName  string // for tool_use
	ToolID    string
	Input     string // JSON string of input, for tool_use
	ResultStr string // for tool_result
}

type ContextMessageView struct {
	Role       string
	Content    string
	RawContent string
	Tokens     int
	ToolBlocks []ToolBlockView
	Timestamp  string
	ReadBadge  string
}

type ContextInspectionView struct {
	Label        string
	SystemPrompt string
	Messages     []ContextMessageView
	Tools        []ToolView
	TotalTokens  int
	MaxTokens    int
	RawJSON      string
}

type ToolView struct {
	Name        string
	Description string
}

type PageData struct {
	Title          string
	Error          string
	Code           string
	TopicID        int64
	Topics         []TopicView
	Messages       []MessageView
	Members        []MemberView
	TopicName      string
	OwnerID        int64
	CurrentUserID  int64
	Skills         []SkillView
	Skill          *SkillView
	Schedules      []ScheduleView
	Schedule       *ScheduleView
	VAPIDPublicKey string
	NavigateTo     string
	PageDataJSON   template.JS
	IsAdmin        bool
	AdminUsers     []AdminUserView
	AdminTopics    []AdminTopicView
	Context        *ContextInspectionView
	BobotHasUnread  bool
	HasOtherUnreads bool
	UnreadJSON      template.JS
}

func (s *Server) render(w http.ResponseWriter, name string, data PageData) {
	data.VAPIDPublicKey = s.cfg.VAPID.PublicKey
	s.templates[name].Execute(w, data)
}

// buildUnreadJSON returns a JSON array of chat IDs with unread messages.
// Private bobot chat is represented as 0.
func buildUnreadJSON(bobotUnread bool, topicUnreads map[int64]bool) template.JS {
	var ids []int64
	if bobotUnread {
		ids = append(ids, 0)
	}
	for topicID := range topicUnreads {
		ids = append(ids, topicID)
	}
	if len(ids) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(ids)
	return template.JS(data)
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

	chatsTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/chats.html")
	if err != nil {
		return err
	}
	s.templates["chats"] = chatsTmpl

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

	skillsTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/skills.html")
	if err != nil {
		return err
	}
	s.templates["skills"] = skillsTmpl

	skillFormTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/skill_form.html")
	if err != nil {
		return err
	}
	s.templates["skill_form"] = skillFormTmpl

	schedulesTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/schedules.html")
	if err != nil {
		return err
	}
	s.templates["schedules"] = schedulesTmpl

	scheduleFormTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/schedule_form.html")
	if err != nil {
		return err
	}
	s.templates["schedule_form"] = scheduleFormTmpl

	adminTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/admin.html")
	if err != nil {
		return err
	}
	s.templates["admin"] = adminTmpl

	adminContextTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/admin_context.html")
	if err != nil {
		return err
	}
	s.templates["admin_context"] = adminContextTmpl

	return nil
}

// validateNavigatePath returns a safe navigation path.
// Only /chat and /chats/{id} are allowed; anything else defaults to /chat.
func validateNavigatePath(path string) string {
	if path == "/chat" || path == "/schedules" || strings.HasPrefix(path, "/schedules?") || path == "/admin" || strings.HasPrefix(path, "/admin/") || navigatePathRe.MatchString(path) {
		return path
	}
	return "/chat"
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	nav := validateNavigatePath(r.URL.Query().Get("navigate"))

	// Check if already authenticated
	if cookie, err := r.Cookie("session"); err == nil {
		if _, err := s.session.DecryptToken(cookie.Value); err == nil {
			s.render(w, "authenticated", PageData{Title: "Loading", NavigateTo: nav})
			return
		}
	}

	// GET request - show login form
	if r.Method == http.MethodGet {
		s.render(w, "login", PageData{Title: "Login", NavigateTo: nav})
		return
	}

	// POST request - handle login
	if err := r.ParseForm(); err != nil {
		s.render(w, "login", PageData{Title: "Login", Error: "Invalid request", NavigateTo: nav})
		return
	}

	// Read navigate from form hidden field (preserved from GET)
	nav = validateNavigatePath(r.FormValue("navigate"))

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.db.GetUserByUsername(username)
	if err != nil {
		s.render(w, "login", PageData{Title: "Login", Error: "Invalid credentials", NavigateTo: nav})
		return
	}

	if !auth.CheckPassword(password, user.PasswordHash) {
		s.render(w, "login", PageData{Title: "Login", Error: "Invalid credentials", NavigateTo: nav})
		return
	}

	if user.Blocked {
		s.render(w, "login", PageData{Title: "Login", Error: "Account blocked", NavigateTo: nav})
		return
	}

	token, err := s.session.CreateToken(user.ID, user.Role)
	if err != nil {
		s.render(w, "login", PageData{Title: "Login", Error: "Internal error", NavigateTo: nav})
		return
	}

	s.setSessionCookie(w, token)
	s.render(w, "authenticated", PageData{Title: "Loading", NavigateTo: nav})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Try to get user from session cookie for cleanup
	if cookie, err := r.Cookie("session"); err == nil {
		if token, err := s.session.DecryptToken(cookie.Value); err == nil {
			// Check for "logout everywhere" parameter
			if r.URL.Query().Get("all") == "true" {
				s.db.CreateSessionRevocation(token.UserID, "logout_all")
			}
			// Clean up push subscriptions
			s.db.DeletePushSubscriptionsByUser(token.UserID)
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

	s.markChatReadImplicit(userData.UserID, db.PrivateChatTopicID)

	bobotUnread, topicUnreads, _ := s.db.GetUnreadChats(userData.UserID)

	s.render(w, "chat", PageData{
		Title:           "Chat",
		PageDataJSON:    template.JS(jsonData),
		IsAdmin:         userData.Role == "admin",
		HasOtherUnreads: len(topicUnreads) > 0,
		UnreadJSON:      buildUnreadJSON(bobotUnread, topicUnreads),
	})
}

func (s *Server) handleChatsPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topics, err := s.db.GetUserTopics(userData.UserID)
	if err != nil {
		http.Error(w, "failed to load topics", http.StatusInternalServerError)
		return
	}

	bobotUnread, topicUnreads, _ := s.db.GetUnreadChats(userData.UserID)

	topicViews := make([]TopicView, 0, len(topics))
	for _, t := range topics {
		members, _ := s.db.GetTopicMembers(t.ID)
		topicViews = append(topicViews, TopicView{
			ID:          t.ID,
			Name:        t.Name,
			MemberCount: len(members),
			HasUnread:   topicUnreads[t.ID],
		})
	}

	s.render(w, "chats", PageData{
		Title:          "Chats",
		Topics:         topicViews,
		BobotHasUnread: bobotUnread,
		UnreadJSON:     buildUnreadJSON(bobotUnread, topicUnreads),
	})
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

	s.markChatReadImplicit(userData.UserID, topicID)

	bobotUnread, topicUnreads, _ := s.db.GetUnreadChats(userData.UserID)
	otherTopicUnreads := len(topicUnreads)
	if topicUnreads[topicID] {
		otherTopicUnreads--
	}

	s.render(w, "topic_chat", PageData{
		Title:           "Topic Chat",
		TopicID:         topicID,
		TopicName:       topic.Name,
		OwnerID:         topic.OwnerID,
		CurrentUserID:   userData.UserID,
		Members:         members,
		PageDataJSON:    template.JS(jsonData),
		IsAdmin:         userData.Role == "admin",
		HasOtherUnreads: bobotUnread || otherTopicUnreads > 0,
		UnreadJSON:      buildUnreadJSON(bobotUnread, topicUnreads),
	})
}
