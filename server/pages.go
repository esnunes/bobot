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
	"github.com/esnunes/bobot/i18n"
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

type QuickActionView struct {
	ID      int64
	Label   string
	Message string
	Mode    string
}

type CalendarPickView struct {
	ID       string
	Name     string
	Timezone string
	Primary  bool
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
	Lang            string
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
	QuickActions    []QuickActionView
	QuickAction     *QuickActionView
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
	IsBobotTopic           bool
	DisplayName            string
	SupportedLanguages     []string
	UserLanguage           string
	GoogleCalendarEnabled  bool
	CalendarConnected      bool
	CalendarName           string
	CalendarsPick          []CalendarPickView
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data PageData) {
	data.VAPIDPublicKey = s.cfg.VAPID.PublicKey
	if data.Lang == "" {
		// Try to get language from auth context (authenticated pages)
		userData := auth.UserDataFromContext(r.Context())
		if userData.Language != "" {
			data.Lang = userData.Language
		} else {
			// Fall back to Accept-Language header (unauthenticated pages)
			data.Lang = i18n.MatchLanguage(r.Header.Get("Accept-Language"))
		}
	}
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
	funcMap := i18n.FuncMap()

	parseTemplate := func(files ...string) (*template.Template, error) {
		return template.New("layout.html").Funcs(funcMap).ParseFS(web.FS, files...)
	}

	templateDefs := map[string]string{
		"landing":           "templates/landing.html",
		"privacy":           "templates/privacy.html",
		"login":             "templates/login.html",
		"signup":            "templates/signup.html",
		"chats":             "templates/chats.html",
		"topic_chat":        "templates/topic_chat.html",
		"authenticated":     "templates/authenticated.html",
		"skills":            "templates/skills.html",
		"skill_form":        "templates/skill_form.html",
		"schedules":         "templates/schedules.html",
		"schedule_form":     "templates/schedule_form.html",
		"admin":             "templates/admin.html",
		"admin_user":        "templates/admin_user.html",
		"settings":          "templates/settings.html",
		"admin_context":     "templates/admin_context.html",
		"quick_actions":     "templates/quick_actions.html",
		"quick_action_form": "templates/quick_action_form.html",
		"calendar_pick":     "templates/calendar_pick.html",
	}

	for name, contentFile := range templateDefs {
		tmpl, err := parseTemplate("templates/layout.html", contentFile)
		if err != nil {
			return err
		}
		s.templates[name] = tmpl
	}

	return nil
}

// validateNavigatePath returns a safe navigation path.
// Only /chat and /chats/{id} are allowed; anything else defaults to /chat.
func validateNavigatePath(path string) string {
	if path == "/chat" || path == "/settings" || strings.HasPrefix(path, "/settings?") || path == "/schedules" || strings.HasPrefix(path, "/schedules?") || path == "/quickactions" || strings.HasPrefix(path, "/quickactions?") || path == "/admin" || strings.HasPrefix(path, "/admin/") || navigatePathRe.MatchString(path) {
		return path
	}
	return "/chat"
}

func (s *Server) handleLandingPage(w http.ResponseWriter, r *http.Request) {
	// Check for existing session — 302 redirect if authenticated
	if cookie, err := r.Cookie("session"); err == nil {
		if _, err := s.session.DecryptToken(cookie.Value); err == nil {
			navigateTo := validateNavigatePath(r.URL.Query().Get("navigate"))
			http.Redirect(w, r, navigateTo, http.StatusFound)
			return
		}
	}

	s.render(w, r, "landing", PageData{Title: "Home"})
}

func (s *Server) handlePrivacyPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "privacy", PageData{Title: "Privacy Policy"})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	nav := validateNavigatePath(r.URL.Query().Get("navigate"))

	// Check if already authenticated
	if cookie, err := r.Cookie("session"); err == nil {
		if _, err := s.session.DecryptToken(cookie.Value); err == nil {
			s.render(w, r, "authenticated", PageData{Title: "Loading", NavigateTo: nav})
			return
		}
	}

	// GET request - show login form
	if r.Method == http.MethodGet {
		s.render(w, r, "login", PageData{Title: "Login", NavigateTo: nav})
		return
	}

	// POST request - handle login
	if err := r.ParseForm(); err != nil {
		s.render(w, r, "login", PageData{Title: "Login", Error: i18n.T(i18n.MatchLanguage(r.Header.Get("Accept-Language")), "login.error.invalid_request"), NavigateTo: nav})
		return
	}

	// Read navigate from form hidden field (preserved from GET)
	nav = validateNavigatePath(r.FormValue("navigate"))
	lang := i18n.MatchLanguage(r.Header.Get("Accept-Language"))

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.db.GetUserByUsername(username)
	if err != nil {
		s.render(w, r, "login", PageData{Title: "Login", Error: i18n.T(lang, "login.error.invalid_credentials"), NavigateTo: nav})
		return
	}

	if !auth.CheckPassword(password, user.PasswordHash) {
		s.render(w, r, "login", PageData{Title: "Login", Error: i18n.T(lang, "login.error.invalid_credentials"), NavigateTo: nav})
		return
	}

	if user.Blocked {
		s.render(w, r, "login", PageData{Title: "Login", Error: i18n.T(lang, "login.error.account_blocked"), NavigateTo: nav})
		return
	}

	token, err := s.session.CreateToken(user.ID, user.Role, user.Language)
	if err != nil {
		s.render(w, r, "login", PageData{Title: "Login", Error: i18n.T(lang, "login.error.internal"), NavigateTo: nav})
		return
	}

	s.setSessionCookie(w, token)
	s.render(w, r, "authenticated", PageData{Title: "Loading", NavigateTo: nav})
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

	w.Header().Set("HX-Location", "/login")
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

	s.render(w, r, "chats", PageData{
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

	// Load quick actions for chat overlay
	qaRows, _ := s.db.GetTopicQuickActions(topicID)
	type quickActionJSON struct {
		Label   string `json:"label"`
		Message string `json:"message"`
		Mode    string `json:"mode"`
	}
	jsonQuickActions := make([]quickActionJSON, 0, len(qaRows))
	for _, qa := range qaRows {
		jsonQuickActions = append(jsonQuickActions, quickActionJSON{
			Label:   qa.Label,
			Message: qa.Message,
			Mode:    qa.Mode,
		})
	}

	canManageQA := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID) == nil

	jsonData, _ := json.Marshal(map[string]any{
		"current_user_id":          userData.UserID,
		"messages":                 jsonMessages,
		"auto_respond":             topic.AutoRespond,
		"quick_actions":            jsonQuickActions,
		"can_manage_quick_actions": canManageQA,
	})

	s.markChatReadImplicit(userData.UserID, topicID)

	unreads, _ := s.db.GetUnreadChats(userData.UserID)
	otherUnreads := len(unreads)
	if unreads[topicID] {
		otherUnreads--
	}

	s.render(w, r, "topic_chat", PageData{
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
		Title:              "Settings",
		CurrentUserID:      userData.UserID,
		IsAdmin:            userData.Role == "admin",
		DisplayName:        user.DisplayName,
		SupportedLanguages: i18n.SupportedLanguages(),
		UserLanguage:       user.Language,
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
		// Load quick actions for inline preview
		qaRows, _ := s.db.GetTopicQuickActions(topicID)
		qaViews := make([]QuickActionView, 0, len(qaRows))
		for _, qa := range qaRows {
			qaViews = append(qaViews, QuickActionView{
				ID:    qa.ID,
				Label: qa.Label,
			})
		}

		data.Skills = skillViews
		data.Schedules = scheduleViews
		data.QuickActions = qaViews

		// Load calendar status
		if s.calendarTool != nil {
			data.GoogleCalendarEnabled = true
			cal, _ := s.calendarTool.DB().GetTopicCalendar(topicID)
			if cal != nil {
				data.CalendarConnected = true
				data.CalendarName = cal.CalendarName
			}
		}
	}

	unreads, _ := s.db.GetUnreadChats(userData.UserID)
	data.UnreadJSON = buildUnreadJSON(unreads)

	s.render(w, r, "settings", data)
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

func (s *Server) handleUpdateLanguage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	lang := strings.TrimSpace(r.FormValue("language"))
	if !i18n.IsSupported(lang) {
		http.Error(w, "unsupported language", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateUserLanguage(userData.UserID, lang); err != nil {
		http.Error(w, "failed to update language", http.StatusInternalServerError)
		return
	}

	// Reissue session token with new language
	newToken, err := s.session.CreateToken(userData.UserID, userData.Role, lang)
	if err == nil {
		s.setSessionCookie(w, newToken)
	}

	// Force full page reload to apply new language
	w.Header().Set("HX-Redirect", r.Header.Get("HX-Current-URL"))
	w.WriteHeader(http.StatusNoContent)
}
