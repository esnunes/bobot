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
	ID            int64
	DisplayName   string
	Username      string
	Role          string
	Blocked       bool
	CreatedAt     string
	LastMessageAt string
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
	SenderName string
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
	Members      []MemberView
	TotalTokens  int
	MaxTokens    int
	RawJSON      string
	BackURL      string
}

type AdminUserTopicView struct {
	ID          int64
	Name        string
	IsOwner     bool
	MemberCount int
	AutoRespond bool
	CreatedAt   string
}

type AdminUserSkillView struct {
	Name        string
	Description string
	TopicName   string
}

type AdminUserPushSubView struct {
	Endpoint  string
	CreatedAt string
}

type AdminUserReadStatusView struct {
	TopicName         string
	LastReadMessageID int64
	ReadAt            string
}

type AdminUserDetailView struct {
	ID            int64
	DisplayName   string
	Username      string
	Role          string
	Blocked       bool
	CreatedAt     string
	LastMessageAt string
	MessageCount  int
	Topics        []AdminUserTopicView
	Profile       string
	Skills        []AdminUserSkillView
	PushSubs      []AdminUserPushSubView
	ReadStatus    []AdminUserReadStatusView
}

type ToolView struct {
	Name        string
	Description string
}

type PageData struct {
	Title           string
	Error           string
	Code            string
	TopicID         int64
	Topics          []TopicView
	Messages        []MessageView
	Members         []MemberView
	TopicName       string
	OwnerID         int64
	CurrentUserID   int64
	Skills          []SkillView
	Skill           *SkillView
	Schedules       []ScheduleView
	Schedule        *ScheduleView
	VAPIDPublicKey  string
	NavigateTo      string
	PageDataJSON    template.JS
	IsAdmin         bool
	AdminUsers      []AdminUserView
	AdminUserDetail *AdminUserDetailView
	Context         *ContextInspectionView
	HasOtherUnreads bool
	UnreadJSON      template.JS
	PushMuted       bool
	AutoRead        bool
	AutoRespond     bool
	IsBobotTopic    bool
	DisplayName     string
}

func (s *Server) render(w http.ResponseWriter, name string, data PageData) {
	data.VAPIDPublicKey = s.cfg.VAPID.PublicKey
	s.templates[name].Execute(w, data)
}

// buildUnreadJSON returns a JSON array of topic IDs with unread messages.
func buildUnreadJSON(unreads map[int64]bool) template.JS {
	var ids []int64
	for topicID := range unreads {
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

	adminUserTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/admin_user.html")
	if err != nil {
		return err
	}
	s.templates["admin_user"] = adminUserTmpl

	settingsTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/settings.html")
	if err != nil {
		return err
	}
	s.templates["settings"] = settingsTmpl

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
	if path == "/chat" || path == "/settings" || strings.HasPrefix(path, "/settings?") || path == "/schedules" || strings.HasPrefix(path, "/schedules?") || path == "/admin" || strings.HasPrefix(path, "/admin/") || navigatePathRe.MatchString(path) {
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

	topic, err := s.db.GetUserBobotTopic(userData.UserID)

	target := "/chats"
	if err == nil && topic != nil {
		target = "/chats/" + strconv.FormatInt(topic.ID, 10)
	}
	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+target+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChatsPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topics, err := s.db.GetUserTopics(userData.UserID)
	if err != nil {
		http.Error(w, "failed to load topics", http.StatusInternalServerError)
		return
	}

	unreads, _ := s.db.GetUnreadChats(userData.UserID)

	topicViews := make([]TopicView, 0, len(topics))
	for _, t := range topics {
		members, _ := s.db.GetTopicMembers(t.ID)
		topicViews = append(topicViews, TopicView{
			ID:          t.ID,
			Name:        t.Name,
			MemberCount: len(members),
			HasUnread:   unreads[t.ID],
		})
	}

	s.render(w, "chats", PageData{
		Title:      "Chats",
		Topics:     topicViews,
		UnreadJSON: buildUnreadJSON(unreads),
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
	senderMap := make(map[int64]*db.TopicMember)
	for i := range dbMembers {
		senderMap[dbMembers[i].UserID] = &dbMembers[i]
	}
	members := make([]MemberView, 0, len(dbMembers))
	var pushMuted bool
	var autoRead bool
	for _, m := range dbMembers {
		members = append(members, MemberView{
			UserID:      m.UserID,
			Username:    m.Username,
			DisplayName: m.DisplayName,
		})
		if m.UserID == userData.UserID {
			pushMuted = m.Muted
			autoRead = m.AutoRead
		}
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
			if member, ok := senderMap[m.SenderID]; ok {
				jm.UserID = member.UserID
				jm.DisplayName = member.DisplayName
			}
		case "assistant", "system":
			jm.DisplayName = "bobot"
		}
		jsonMessages = append(jsonMessages, jm)
	}

	jsonData, _ := json.Marshal(map[string]any{
		"current_user_id": userData.UserID,
		"messages":        jsonMessages,
		"auto_respond":    topic.AutoRespond,
	})

	s.markChatReadImplicit(userData.UserID, topicID)

	unreads, _ := s.db.GetUnreadChats(userData.UserID)
	otherUnreads := len(unreads)
	if unreads[topicID] {
		otherUnreads--
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
		HasOtherUnreads: otherUnreads > 0,
		UnreadJSON:      buildUnreadJSON(unreads),
		PushMuted:       pushMuted,
		AutoRead:        autoRead,
		AutoRespond:     topic.AutoRespond,
	})
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	user, err := s.db.GetUserByID(userData.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}

	data := PageData{
		Title:         "Settings",
		CurrentUserID: userData.UserID,
		IsAdmin:       userData.Role == "admin",
		DisplayName:   user.DisplayName,
	}

	// If topic_id is provided, load topic-specific data
	topicIDStr := r.URL.Query().Get("topic_id")
	if topicIDStr != "" {
		topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}

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
		var pushMuted bool
		var autoRead bool
		for _, m := range dbMembers {
			members = append(members, MemberView{
				UserID:      m.UserID,
				Username:    m.Username,
				DisplayName: m.DisplayName,
			})
			if m.UserID == userData.UserID {
				pushMuted = m.Muted
				autoRead = m.AutoRead
			}
		}

		// Load skills for inline preview
		skills, _ := s.db.GetTopicSkills(topicID)
		skillViews := make([]SkillView, 0, len(skills))
		for _, sk := range skills {
			skillViews = append(skillViews, SkillView{
				ID:   sk.ID,
				Name: sk.Name,
			})
		}

		// Load schedules for inline preview
		var scheduleViews []ScheduleView
		if s.scheduleDB != nil {
			jobs, _ := s.scheduleDB.ListCronJobsByTopic(topicID)
			scheduleViews = make([]ScheduleView, 0, len(jobs))
			for _, j := range jobs {
				scheduleViews = append(scheduleViews, ScheduleView{
					ID:       j.ID,
					Name:     j.Name,
					CronExpr: j.CronExpr,
					Enabled:  j.Enabled,
				})
			}
		}

		data.TopicID = topicID
		data.TopicName = topic.Name
		data.OwnerID = topic.OwnerID
		data.Members = members
		data.PushMuted = pushMuted
		data.AutoRead = autoRead
		data.AutoRespond = topic.AutoRespond
		data.IsBobotTopic = topic.Name == "bobot"
		data.Skills = skillViews
		data.Schedules = scheduleViews
	}

	unreads, _ := s.db.GetUnreadChats(userData.UserID)
	data.UnreadJSON = buildUnreadJSON(unreads)

	s.render(w, "settings", data)
}

func (s *Server) handleUpdateDisplayName(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	if len(displayName) < 1 || len(displayName) > 100 {
		http.Error(w, "display name must be 1-100 characters", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateUserDisplayName(userData.UserID, displayName); err != nil {
		http.Error(w, "failed to update display name", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
