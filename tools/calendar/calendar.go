// tools/calendar/calendar.go
package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
	"golang.org/x/oauth2"
)

type CalendarTool struct {
	db    *CalendarDB
	oauth *OAuthConfig
}

func NewCalendarTool(db *CalendarDB, clientID, clientSecret, baseURL string) *CalendarTool {
	return &CalendarTool{
		db:    db,
		oauth: NewOAuthConfig(clientID, clientSecret, baseURL, db),
	}
}

func (t *CalendarTool) Name() string    { return "calendar" }
func (t *CalendarTool) AdminOnly() bool { return false }

func (t *CalendarTool) Description() string {
	return "Manage Google Calendar events for this topic. List upcoming events, create new events, update existing events, and delete events. The calendar must be connected by the topic owner in settings first."
}

func (t *CalendarTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "create", "update", "delete"},
				"description": "The calendar operation to perform",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Event title (required for create, used for matching on update/delete)",
			},
			"start": map[string]any{
				"type":        "string",
				"description": "Start date/time in RFC3339 or YYYY-MM-DD or YYYY-MM-DDTHH:MM format",
			},
			"end": map[string]any{
				"type":        "string",
				"description": "End date/time in RFC3339 or YYYY-MM-DD or YYYY-MM-DDTHH:MM format",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Event description (optional)",
			},
			"location": map[string]any{
				"type":        "string",
				"description": "Event location (optional)",
			},
			"event_id": map[string]any{
				"type":        "string",
				"description": "Google Calendar event ID (for update/delete when title is ambiguous)",
			},
			"start_date": map[string]any{
				"type":        "string",
				"description": "Start of date range for list (default: today). Format: YYYY-MM-DD",
			},
			"end_date": map[string]any{
				"type":        "string",
				"description": "End of date range for list (default: 7 days from start). Format: YYYY-MM-DD",
			},
		},
		"required": []string{"command"},
	}
}

func (t *CalendarTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing arguments. Usage: /calendar <list|create|update|delete> [options]")
	}

	command := parts[0]
	result := map[string]any{"command": command}

	switch command {
	case "list":
		if len(parts) > 1 {
			result["start_date"] = parts[1]
		}
		if len(parts) > 2 {
			result["end_date"] = parts[2]
		}
		return result, nil

	case "create":
		if len(parts) < 4 {
			return nil, fmt.Errorf("usage: /calendar create <title> <start> <end> [description]")
		}
		result["title"] = parts[1]
		result["start"] = parts[2]
		result["end"] = parts[3]
		if len(parts) > 4 {
			result["description"] = strings.Join(parts[4:], " ")
		}
		return result, nil

	case "update":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /calendar update <event_id> [--title=...] [--start=...] [--end=...]")
		}
		result["event_id"] = parts[1]
		for _, p := range parts[2:] {
			if strings.HasPrefix(p, "--title=") {
				result["title"] = strings.TrimPrefix(p, "--title=")
			} else if strings.HasPrefix(p, "--start=") {
				result["start"] = strings.TrimPrefix(p, "--start=")
			} else if strings.HasPrefix(p, "--end=") {
				result["end"] = strings.TrimPrefix(p, "--end=")
			} else if strings.HasPrefix(p, "--description=") {
				result["description"] = strings.TrimPrefix(p, "--description=")
			}
		}
		return result, nil

	case "delete":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /calendar delete <event_id>")
		}
		result["event_id"] = parts[1]
		return result, nil

	default:
		return nil, fmt.Errorf("unknown command: %s. Available: list, create, update, delete", command)
	}
}

func (t *CalendarTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("missing command")
	}

	chatData := auth.ChatDataFromContext(ctx)
	topicID := chatData.TopicID
	if topicID == 0 {
		return "", fmt.Errorf("calendar tool requires a topic context")
	}

	// Check if calendar is connected
	cal, err := t.db.GetTopicCalendar(topicID)
	if err != nil {
		return "", fmt.Errorf("checking calendar: %w", err)
	}
	if cal == nil {
		return "No Google Calendar is connected to this topic. The topic owner can connect one from the topic settings page.", nil
	}

	// Get token source
	ts, err := t.oauth.GetTokenSource(topicID)
	if err != nil {
		return "", fmt.Errorf("getting calendar access: %w", err)
	}
	if ts == nil {
		return "Google Calendar tokens are missing. The topic owner needs to reconnect from settings.", nil
	}

	switch command {
	case "list":
		return t.execList(ctx, ts, cal, input)
	case "create":
		return t.execCreate(ctx, ts, cal, input)
	case "update":
		return t.execUpdate(ctx, ts, cal, input)
	case "delete":
		return t.execDelete(ctx, ts, cal, input)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *CalendarTool) execList(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, input map[string]any) (string, error) {
	now := time.Now()
	timeMin := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	timeMax := timeMin.AddDate(0, 0, 7)

	if startDate, ok := input["start_date"].(string); ok && startDate != "" {
		if parsed, err := time.Parse("2006-01-02", startDate); err == nil {
			timeMin = parsed
			timeMax = timeMin.AddDate(0, 0, 7)
		}
	}
	if endDate, ok := input["end_date"].(string); ok && endDate != "" {
		if parsed, err := time.Parse("2006-01-02", endDate); err == nil {
			timeMax = parsed.Add(24 * time.Hour)
		}
	}

	events, err := ListEvents(ctx, ts, cal.CalendarID, timeMin, timeMax, cal.Timezone)
	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return fmt.Sprintf("No events found between %s and %s (timezone: %s).",
			timeMin.Format("2006-01-02"), timeMax.Add(-1).Format("2006-01-02"), cal.Timezone), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Events from %s to %s (timezone: %s):\n\n",
		timeMin.Format("2006-01-02"), timeMax.Add(-time.Second).Format("2006-01-02"), cal.Timezone))

	for _, e := range events {
		if e.AllDay {
			sb.WriteString(fmt.Sprintf("- **%s** (all day, %s)", e.Title, e.Start))
		} else {
			start := formatEventTime(e.Start)
			end := formatEventTime(e.End)
			sb.WriteString(fmt.Sprintf("- **%s** (%s - %s)", e.Title, start, end))
		}
		sb.WriteString(fmt.Sprintf(" [ID: %s]\n", e.ID))
		if e.Location != "" {
			sb.WriteString(fmt.Sprintf("  Location: %s\n", e.Location))
		}
		if e.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", e.Description))
		}
		if e.HTMLLink != "" {
			sb.WriteString(fmt.Sprintf("  Link: %s\n", e.HTMLLink))
		}
	}

	return sb.String(), nil
}

func (t *CalendarTool) execCreate(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, input map[string]any) (string, error) {
	title, _ := input["title"].(string)
	if title == "" {
		return "", fmt.Errorf("title is required for creating an event")
	}

	startStr, _ := input["start"].(string)
	if startStr == "" {
		return "", fmt.Errorf("start time is required")
	}

	endStr, _ := input["end"].(string)

	description, _ := input["description"].(string)
	location, _ := input["location"].(string)

	startTime, err := parseFlexibleTime(startStr, cal.Timezone)
	if err != nil {
		return "", fmt.Errorf("invalid start time: %w", err)
	}

	// Determine if all-day event
	allDay := len(startStr) == 10 // YYYY-MM-DD format

	var endTime time.Time
	if endStr != "" {
		endTime, err = parseFlexibleTime(endStr, cal.Timezone)
		if err != nil {
			return "", fmt.Errorf("invalid end time: %w", err)
		}
	} else if allDay {
		endTime = startTime
	} else {
		endTime = startTime.Add(time.Hour) // Default 1 hour
	}

	event, err := CreateEvent(ctx, ts, cal.CalendarID, title, description, location, startTime, endTime, allDay, cal.Timezone)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Event created: **%s**\n", event.Title))
	if event.AllDay {
		sb.WriteString(fmt.Sprintf("Date: %s (all day)\n", event.Start))
	} else {
		sb.WriteString(fmt.Sprintf("Time: %s - %s\n", formatEventTime(event.Start), formatEventTime(event.End)))
	}
	if event.Location != "" {
		sb.WriteString(fmt.Sprintf("Location: %s\n", event.Location))
	}
	sb.WriteString(fmt.Sprintf("ID: %s", event.ID))

	return sb.String(), nil
}

func (t *CalendarTool) execUpdate(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, input map[string]any) (string, error) {
	eventID, _ := input["event_id"].(string)
	if eventID == "" {
		// Try to find by title
		title, _ := input["title"].(string)
		if title == "" {
			return "", fmt.Errorf("event_id or title is required for update")
		}
		matches, err := FindEventByTitle(ctx, ts, cal.CalendarID, title)
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			return fmt.Sprintf("No event found matching '%s'.", title), nil
		}
		if len(matches) > 1 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Multiple events match '%s'. Please specify event_id:\n", title))
			for _, m := range matches {
				sb.WriteString(fmt.Sprintf("- %s (%s) [ID: %s]\n", m.Title, formatEventTime(m.Start), m.ID))
			}
			return sb.String(), nil
		}
		eventID = matches[0].ID
	}

	updates := make(map[string]any)
	if title, ok := input["title"].(string); ok {
		updates["title"] = title
	}
	if start, ok := input["start"].(string); ok {
		updates["start"] = start
	}
	if end, ok := input["end"].(string); ok {
		updates["end"] = end
	}
	if desc, ok := input["description"].(string); ok {
		updates["description"] = desc
	}
	if loc, ok := input["location"].(string); ok {
		updates["location"] = loc
	}

	event, err := UpdateEvent(ctx, ts, cal.CalendarID, eventID, updates, cal.Timezone)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Event updated: **%s** (%s - %s) [ID: %s]",
		event.Title, formatEventTime(event.Start), formatEventTime(event.End), event.ID), nil
}

func (t *CalendarTool) execDelete(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, input map[string]any) (string, error) {
	eventID, _ := input["event_id"].(string)
	if eventID == "" {
		// Try to find by title
		title, _ := input["title"].(string)
		if title == "" {
			return "", fmt.Errorf("event_id or title is required for delete")
		}
		matches, err := FindEventByTitle(ctx, ts, cal.CalendarID, title)
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			return fmt.Sprintf("No event found matching '%s'.", title), nil
		}
		if len(matches) > 1 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Multiple events match '%s'. Please specify event_id:\n", title))
			for _, m := range matches {
				sb.WriteString(fmt.Sprintf("- %s (%s) [ID: %s]\n", m.Title, formatEventTime(m.Start), m.ID))
			}
			return sb.String(), nil
		}
		eventID = matches[0].ID
	}

	if err := DeleteEvent(ctx, ts, cal.CalendarID, eventID); err != nil {
		return "", err
	}

	return fmt.Sprintf("Event deleted successfully. [ID: %s]", eventID), nil
}

func formatEventTime(s string) string {
	if s == "" {
		return ""
	}
	// Try to parse RFC3339 and format more human-readable
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("Mon Jan 2, 3:04 PM")
	}
	return s
}

// DB returns the calendar database for use by server handlers.
func (t *CalendarTool) DB() *CalendarDB {
	return t.db
}

// OAuth returns the OAuth config for use by server handlers.
func (t *CalendarTool) OAuth() *OAuthConfig {
	return t.oauth
}
