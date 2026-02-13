package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/tools/schedule"
)

func (s *Server) handleSchedulesPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicIDStr := r.URL.Query().Get("topic_id")

	var jobs []schedule.CronJob
	var err error
	var topicID int64
	var topicName string

	if topicIDStr != "" {
		topicID, err = strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		isMember, memberErr := s.db.IsTopicMember(topicID, userData.UserID)
		if memberErr != nil || !isMember {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		topic, topicErr := s.db.GetTopicByID(topicID)
		if topicErr != nil {
			http.Error(w, "topic not found", http.StatusNotFound)
			return
		}
		topicName = topic.Name
		jobs, err = s.scheduleDB.ListCronJobsByTopic(topicID)
	} else {
		jobs, err = s.scheduleDB.ListCronJobs(userData.UserID)
	}

	if err != nil {
		http.Error(w, "failed to load schedules", http.StatusInternalServerError)
		return
	}

	scheduleViews := make([]ScheduleView, 0, len(jobs))
	for _, j := range jobs {
		preview := j.Prompt
		if len([]rune(preview)) > 50 {
			preview = string([]rune(preview)[:50]) + "..."
		}
		scheduleViews = append(scheduleViews, ScheduleView{
			ID:           j.ID,
			Name:         j.Name,
			Prompt:       j.Prompt,
			PromptPreview: preview,
			CronExpr:     j.CronExpr,
			Enabled:      j.Enabled,
			NextRunAt:    j.NextRunAt.Format("2006-01-02 15:04 UTC"),
		})
	}

	s.render(w, "schedules", PageData{
		Title:     "Schedules",
		TopicID:   topicID,
		TopicName: topicName,
		Schedules: scheduleViews,
	})
}

func (s *Server) handleScheduleFormPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicIDStr := r.URL.Query().Get("topic_id")
	var topicID int64
	if topicIDStr != "" {
		var err error
		topicID, err = strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
	}

	// Check if editing
	idStr := r.PathValue("id")
	if idStr != "" {
		scheduleID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid schedule id", http.StatusBadRequest)
			return
		}

		job, err := s.scheduleDB.GetCronJob(scheduleID)
		if err == schedule.ErrNotFound {
			http.Error(w, "schedule not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to load schedule", http.StatusInternalServerError)
			return
		}

		// Verify ownership
		if job.UserID != userData.UserID && userData.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if job.TopicID != nil {
			topicID = *job.TopicID
		}

		s.render(w, "schedule_form", PageData{
			Title:   "Edit Schedule",
			TopicID: topicID,
			Schedule: &ScheduleView{
				ID:       job.ID,
				Name:     job.Name,
				Prompt:   job.Prompt,
				CronExpr: job.CronExpr,
				Enabled:  job.Enabled,
			},
		})
		return
	}

	s.render(w, "schedule_form", PageData{
		Title:   "New Schedule",
		TopicID: topicID,
	})
}

func (s *Server) handleCreateScheduleForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	prompt := strings.TrimSpace(r.FormValue("prompt"))
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}

	cronExprStr := strings.TrimSpace(r.FormValue("cron_expr"))
	if cronExprStr == "" {
		http.Error(w, "cron expression required", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	topicIDStr := r.FormValue("topic_id")

	// Validate cron expression
	expr, err := schedule.Parse(cronExprStr)
	if err != nil {
		s.render(w, "schedule_form", PageData{
			Title: "New Schedule",
			Error: fmt.Sprintf("Invalid cron expression: %v", err),
		})
		return
	}

	// Check max jobs
	count, _ := s.scheduleDB.CountEnabledCronJobs(userData.UserID)
	if count >= s.cfg.Schedule.MaxCronJobs {
		s.render(w, "schedule_form", PageData{
			Title: "New Schedule",
			Error: fmt.Sprintf("Maximum of %d active schedules reached", s.cfg.Schedule.MaxCronJobs),
		})
		return
	}

	var topicID *int64
	redirectPath := "/schedules"

	if topicIDStr != "" {
		tid, err := strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		isMember, memberErr := s.db.IsTopicMember(tid, userData.UserID)
		if memberErr != nil || !isMember {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		topicID = &tid
		redirectPath = fmt.Sprintf("/schedules?topic_id=%d", tid)
	}

	nextRunAt := expr.Next(time.Now().UTC())
	_, err = s.scheduleDB.CreateCronJob(userData.UserID, topicID, name, prompt, cronExprStr, nextRunAt)
	if err != nil {
		http.Error(w, "failed to create schedule", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUpdateScheduleForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	scheduleID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid schedule id", http.StatusBadRequest)
		return
	}

	job, err := s.scheduleDB.GetCronJob(scheduleID)
	if err == schedule.ErrNotFound {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load schedule", http.StatusInternalServerError)
		return
	}

	if job.UserID != userData.UserID && userData.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	prompt := strings.TrimSpace(r.FormValue("prompt"))
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}

	cronExprStr := strings.TrimSpace(r.FormValue("cron_expr"))
	if cronExprStr == "" {
		http.Error(w, "cron expression required", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	enabled := r.FormValue("enabled") == "on"

	// Validate cron expression
	expr, err := schedule.Parse(cronExprStr)
	if err != nil {
		var topicID int64
		if job.TopicID != nil {
			topicID = *job.TopicID
		}
		s.render(w, "schedule_form", PageData{
			Title:   "Edit Schedule",
			TopicID: topicID,
			Error:   fmt.Sprintf("Invalid cron expression: %v", err),
			Schedule: &ScheduleView{
				ID:       job.ID,
				Name:     name,
				Prompt:   prompt,
				CronExpr: cronExprStr,
				Enabled:  enabled,
			},
		})
		return
	}

	nextRunAt := expr.Next(time.Now().UTC())
	if err := s.scheduleDB.UpdateCronJob(scheduleID, userData.UserID, name, prompt, cronExprStr, enabled, nextRunAt); err != nil {
		http.Error(w, "failed to update schedule", http.StatusInternalServerError)
		return
	}

	redirectPath := "/schedules"
	if job.TopicID != nil {
		redirectPath = fmt.Sprintf("/schedules?topic_id=%d", *job.TopicID)
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteScheduleForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	scheduleID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid schedule id", http.StatusBadRequest)
		return
	}

	job, err := s.scheduleDB.GetCronJob(scheduleID)
	if err == schedule.ErrNotFound {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load schedule", http.StatusInternalServerError)
		return
	}

	if job.UserID != userData.UserID && userData.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.scheduleDB.DeleteCronJob(scheduleID, userData.UserID); err != nil {
		http.Error(w, "failed to delete schedule", http.StatusInternalServerError)
		return
	}

	redirectPath := "/schedules"
	if job.TopicID != nil {
		redirectPath = fmt.Sprintf("/schedules?topic_id=%d", *job.TopicID)
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}
