// server/calendar.go
package server

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/tools/calendar"
)

func (s *Server) handleCalendarAuth(w http.ResponseWriter, r *http.Request) {
	if s.calendarTool == nil {
		http.Error(w, "Google Calendar not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())
	topicIDStr := r.URL.Query().Get("topic_id")
	if topicIDStr == "" {
		http.Error(w, "missing topic_id", http.StatusBadRequest)
		return
	}

	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	if err := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	authURL, err := s.calendarTool.OAuth().GenerateAuthURL(userData.UserID, topicID)
	if err != nil {
		slog.Error("calendar: failed to generate auth URL", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleCalendarCallback(w http.ResponseWriter, r *http.Request) {
	if s.calendarTool == nil {
		http.Error(w, "Google Calendar not configured", http.StatusNotFound)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	topicID, stateUserID, err := s.calendarTool.OAuth().ExchangeCode(r.Context(), code, state)
	if err != nil {
		slog.Error("calendar: OAuth exchange failed", "error", err)
		http.Error(w, "OAuth authorization failed", http.StatusBadRequest)
		return
	}

	// Verify the session user matches the user who initiated the OAuth flow
	userData := auth.UserDataFromContext(r.Context())
	if userData.UserID != stateUserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	http.Redirect(w, r, "/calendar/pick?topic_id="+strconv.FormatInt(topicID, 10), http.StatusFound)
}

func (s *Server) handleCalendarPickPage(w http.ResponseWriter, r *http.Request) {
	if s.calendarTool == nil {
		http.Error(w, "Google Calendar not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())
	topicIDStr := r.URL.Query().Get("topic_id")
	if topicIDStr == "" {
		http.Error(w, "missing topic_id", http.StatusBadRequest)
		return
	}

	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	if err := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ts, err := s.calendarTool.OAuth().GetTokenSource(topicID)
	if err != nil || ts == nil {
		slog.Error("calendar: no token for calendar pick", "topic_id", topicID, "error", err)
		http.Redirect(w, r, "/settings?topic_id="+topicIDStr, http.StatusFound)
		return
	}

	calendars, err := calendar.ListCalendars(r.Context(), ts)
	if err != nil {
		slog.Error("calendar: failed to list calendars", "error", err)
		http.Error(w, "Failed to load calendars from Google", http.StatusInternalServerError)
		return
	}

	calViews := make([]CalendarPickView, 0, len(calendars))
	for _, c := range calendars {
		calViews = append(calViews, CalendarPickView{
			ID:       c.ID,
			Name:     c.Name,
			Timezone: c.Timezone,
			Primary:  c.Primary,
		})
	}

	s.render(w, r, "calendar_pick", PageData{
		Title:          "Pick Calendar",
		TopicID:        topicID,
		CalendarsPick:  calViews,
	})
}

func (s *Server) handleCalendarPickSubmit(w http.ResponseWriter, r *http.Request) {
	if s.calendarTool == nil {
		http.Error(w, "Google Calendar not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	topicIDStr := r.FormValue("topic_id")
	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	if err := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	calendarID := r.FormValue("calendar_id")
	calendarName := r.FormValue("calendar_name")
	timezone := r.FormValue("timezone")

	if calendarID == "" {
		http.Error(w, "please select a calendar", http.StatusBadRequest)
		return
	}

	if err := s.calendarTool.DB().SaveTopicCalendar(calendar.TopicCalendar{
		TopicID:      topicID,
		CalendarID:   calendarID,
		CalendarName: calendarName,
		Timezone:     timezone,
	}); err != nil {
		slog.Error("calendar: failed to save calendar selection", "error", err)
		http.Error(w, "failed to save selection", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings?topic_id="+topicIDStr, http.StatusFound)
}

func (s *Server) handleCalendarDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.calendarTool == nil {
		http.Error(w, "Google Calendar not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())
	topicIDStr := r.URL.Query().Get("topic_id")
	if topicIDStr == "" {
		http.Error(w, "missing topic_id", http.StatusBadRequest)
		return
	}

	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	if err := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Best-effort revocation at Google (continues even if it fails)
	if err := s.calendarTool.OAuth().RevokeToken(topicID); err != nil {
		slog.Error("calendar: revoke failed (continuing with cleanup)", "error", err)
	}

	// Remove local token and calendar data
	if err := s.calendarTool.DB().Disconnect(topicID); err != nil {
		slog.Error("calendar: disconnect failed", "error", err)
		http.Error(w, "failed to disconnect", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
