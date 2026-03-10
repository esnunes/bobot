// tools/calendar/calendar.go
package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
	"golang.org/x/oauth2"
	gcalendar "google.golang.org/api/calendar/v3"
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
	return "Manage Google Calendar events for this topic. Supports one-time and recurring events. " +
		"Use the recurrence parameter with create to set up repeating events (pass an iCal RRULE string). " +
		"For recurring events, use the scope parameter with update/delete to choose: " +
		"'single' (this instance only), 'all' (entire series), or 'this_and_future' (this and all later instances). " +
		"When a user's intent about recurring event scope is ambiguous, ask them to clarify. " +
		"Event IDs from list results can be used directly -- the tool handles instance-vs-series resolution internally."
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
			"recurrence": map[string]any{
				"type": "string",
				"description": "iCal RRULE string for recurring events (optional). " +
					"Examples: RRULE:FREQ=DAILY, RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR, " +
					"RRULE:FREQ=MONTHLY;BYMONTHDAY=1, RRULE:FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=9, " +
					"RRULE:FREQ=WEEKLY;BYDAY=TU;COUNT=10, RRULE:FREQ=DAILY;UNTIL=20261231T235959Z. " +
					"Must start with RRULE: prefix. Used with create to set up recurring events, " +
					"or with update (scope='all' or 'this_and_future') to change the recurrence rule.",
			},
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{"single", "all", "this_and_future"},
				"description": "Scope for update/delete on recurring events. 'single': modify only this instance (default). 'all': modify the entire recurring series. 'this_and_future': modify this instance and all future instances. When the user's intent is ambiguous for a recurring event, ask them to clarify. Ignored for non-recurring events.",
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
			return nil, fmt.Errorf("usage: /calendar create <title> <start> <end> [--recurrence=RRULE:...] [description]")
		}
		result["title"] = parts[1]
		result["start"] = parts[2]
		result["end"] = parts[3]
		var descWords []string
		for _, p := range parts[4:] {
			if strings.HasPrefix(p, "--recurrence=") {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 {
					result["recurrence"] = kv[1]
				}
			} else if strings.HasPrefix(p, "--location=") {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 {
					result["location"] = kv[1]
				}
			} else if !strings.HasPrefix(p, "--") {
				descWords = append(descWords, p)
			}
		}
		if len(descWords) > 0 {
			result["description"] = strings.Join(descWords, " ")
		}
		return result, nil

	case "update":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /calendar update <event_id> [--title=...] [--start=...] [--end=...] [--scope=...]")
		}
		result["event_id"] = parts[1]
		for _, p := range parts[2:] {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 || !strings.HasPrefix(kv[0], "--") {
				continue
			}
			key := strings.TrimPrefix(kv[0], "--")
			switch key {
			case "title":
				result["title"] = kv[1]
			case "start":
				result["start"] = kv[1]
			case "end":
				result["end"] = kv[1]
			case "description":
				result["description"] = kv[1]
			case "location":
				result["location"] = kv[1]
			case "recurrence":
				result["recurrence"] = kv[1]
			case "scope":
				result["scope"] = kv[1]
			}
		}
		return result, nil

	case "delete":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /calendar delete <event_id> [--scope=single|all|this_and_future]")
		}
		result["event_id"] = parts[1]
		for _, p := range parts[2:] {
			if strings.HasPrefix(p, "--scope=") {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 {
					result["scope"] = kv[1]
				}
			}
		}
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
		if e.IsRecurring {
			sb.WriteString(" (recurring)")
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

	var recurrence []string
	if rec, ok := input["recurrence"].(string); ok && rec != "" {
		if err := validateRRULE(rec); err != nil {
			return "", err
		}
		recurrence = []string{rec}
	}

	event, err := CreateEvent(ctx, ts, cal.CalendarID, title, description, location, startTime, endTime, allDay, cal.Timezone, recurrence)
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
	if len(recurrence) > 0 {
		sb.WriteString(fmt.Sprintf("Recurrence: %s\n", recurrence[0]))
	}
	sb.WriteString(fmt.Sprintf("ID: %s", event.ID))

	return sb.String(), nil
}

func (t *CalendarTool) execUpdate(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, input map[string]any) (string, error) {
	eventID, msg, err := t.resolveEventID(ctx, ts, cal, input)
	if err != nil {
		return "", err
	}
	if msg != "" {
		return msg, nil
	}
	if eventID == "" {
		return "", fmt.Errorf("event_id or title is required for update")
	}

	scope := extractScope(input)

	switch scope {
	case "all":
		return t.updateAllInstances(ctx, ts, cal, eventID, input)
	case "this_and_future":
		return t.updateThisAndFuture(ctx, ts, cal, eventID, input)
	default: // "single"
		return t.updateSingleInstance(ctx, ts, cal, eventID, input)
	}
}

func (t *CalendarTool) updateSingleInstance(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, eventID string, input map[string]any) (string, error) {
	// Guard: if the event ID is a recurring master, updating it would affect all
	// instances. Fetch the event to check.
	raw, err := GetRawEvent(ctx, ts, cal.CalendarID, eventID)
	if err != nil {
		return "", err
	}
	if len(raw.Recurrence) > 0 {
		return "This is a recurring event series. To update a single occurrence, first list events " +
			"to get the specific instance ID, then update that instance. " +
			"To update all occurrences, use scope='all'. " +
			"To update this and all future occurrences, use scope='this_and_future'.", nil
	}

	updates := buildUpdates(input)

	event, err := UpdateEvent(ctx, ts, cal.CalendarID, eventID, updates, cal.Timezone)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Event updated: **%s** (%s - %s) [ID: %s]",
		event.Title, formatEventTime(event.Start), formatEventTime(event.End), event.ID), nil
}

func (t *CalendarTool) updateAllInstances(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, eventID string, input map[string]any) (string, error) {
	masterID := masterEventID(eventID)
	updates := buildUpdates(input)

	// Handle recurrence changes on the master event
	if rec, ok := input["recurrence"].(string); ok {
		if rec != "" {
			if err := validateRRULE(rec); err != nil {
				return "", err
			}
		}
		updates["recurrence"] = rec
	}

	// For recurrence changes, we need to use the raw event API
	if _, hasRec := updates["recurrence"]; hasRec {
		raw, err := GetRawEvent(ctx, ts, cal.CalendarID, masterID)
		if err != nil {
			return "", err
		}

		rec := updates["recurrence"].(string)
		if rec == "" {
			// Remove recurrence
			raw.Recurrence = nil
		} else {
			raw.Recurrence = []string{rec}
		}
		delete(updates, "recurrence")

		// Apply other updates to raw event
		applyUpdatesToRaw(raw, updates, cal.Timezone)

		updated, err := UpdateRawEvent(ctx, ts, cal.CalendarID, masterID, raw)
		if err != nil {
			return "", err
		}

		info := eventToInfo(updated)
		return fmt.Sprintf("All instances updated: **%s** [ID: %s]", info.Title, info.ID), nil
	}

	event, err := UpdateEvent(ctx, ts, cal.CalendarID, masterID, updates, cal.Timezone)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("All instances updated: **%s** [ID: %s]", event.Title, event.ID), nil
}

func (t *CalendarTool) updateThisAndFuture(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, eventID string, input map[string]any) (string, error) {
	// Get the instance to find its start time
	instance, err := GetRawEvent(ctx, ts, cal.CalendarID, eventID)
	if err != nil {
		return "", fmt.Errorf("fetching event instance: %w", err)
	}

	masterID := masterEventID(eventID)
	if masterID == eventID && instance.RecurringEventId == "" {
		// Not a recurring event instance, fall back to single update
		return t.updateSingleInstance(ctx, ts, cal, eventID, input)
	}
	if instance.RecurringEventId != "" {
		masterID = instance.RecurringEventId
	}

	// Get the master event
	master, err := GetRawEvent(ctx, ts, cal.CalendarID, masterID)
	if err != nil {
		return "", fmt.Errorf("fetching master event: %w", err)
	}

	// Save original recurrence for rollback
	originalRecurrence := make([]string, len(master.Recurrence))
	copy(originalRecurrence, master.Recurrence)

	// Determine the UNTIL timestamp (1 second before this instance's start)
	untilTime := instanceStartTime(instance)
	if untilTime.IsZero() {
		return "", fmt.Errorf("could not determine instance start time")
	}
	until := untilTime.Add(-time.Second).UTC().Format("20060102T150405Z")

	// Step 1: Trim master RRULE with UNTIL
	trimmedRecurrence := trimRecurrenceUntil(master.Recurrence, until)
	master.Recurrence = trimmedRecurrence
	_, err = UpdateRawEvent(ctx, ts, cal.CalendarID, masterID, master)
	if err != nil {
		return "", fmt.Errorf("trimming recurring series: %w", err)
	}

	// Step 2: Create new series from this instance with changes
	newStart := untilTime
	newEnd := instanceEndTime(instance)
	if newEnd.IsZero() {
		newEnd = newStart.Add(time.Hour)
	}

	title := master.Summary
	description := master.Description
	location := master.Location
	newRecurrence := originalRecurrence

	// Apply user updates
	if t, ok := input["title"].(string); ok && t != "" {
		title = t
	}
	if d, ok := input["description"].(string); ok {
		description = d
	}
	if l, ok := input["location"].(string); ok {
		location = l
	}
	if startStr, ok := input["start"].(string); ok && startStr != "" {
		parsed, err := parseFlexibleTime(startStr, cal.Timezone)
		if err != nil {
			return "", fmt.Errorf("invalid start time: %w", err)
		}
		newStart = parsed
	}
	if endStr, ok := input["end"].(string); ok && endStr != "" {
		parsed, err := parseFlexibleTime(endStr, cal.Timezone)
		if err != nil {
			return "", fmt.Errorf("invalid end time: %w", err)
		}
		newEnd = parsed
	}
	if rec, ok := input["recurrence"].(string); ok && rec != "" {
		if err := validateRRULE(rec); err != nil {
			return "", err
		}
		newRecurrence = []string{rec}
	}

	allDay := instance.Start != nil && instance.Start.Date != ""

	created, err := CreateEvent(ctx, ts, cal.CalendarID, title, description, location, newStart, newEnd, allDay, cal.Timezone, newRecurrence)
	if err != nil {
		// Rollback: restore original master recurrence
		master.Recurrence = originalRecurrence
		_, rollbackErr := UpdateRawEvent(ctx, ts, cal.CalendarID, masterID, master)
		if rollbackErr != nil {
			return "", fmt.Errorf("failed to create new series (%w) and rollback also failed (%w)", err, rollbackErr)
		}
		return "", fmt.Errorf("failed to create new series (original series restored): %w", err)
	}

	return fmt.Sprintf("Updated this and future instances. Original series trimmed, new series created: **%s** [ID: %s]",
		created.Title, created.ID), nil
}

func (t *CalendarTool) execDelete(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, input map[string]any) (string, error) {
	eventID, msg, err := t.resolveEventID(ctx, ts, cal, input)
	if err != nil {
		return "", err
	}
	if msg != "" {
		return msg, nil
	}
	if eventID == "" {
		return "", fmt.Errorf("event_id or title is required for delete")
	}

	scope := extractScope(input)

	switch scope {
	case "all":
		return t.deleteAllInstances(ctx, ts, cal, eventID)
	case "this_and_future":
		return t.deleteThisAndFuture(ctx, ts, cal, eventID)
	default: // "single"
		return t.deleteSingleInstance(ctx, ts, cal, eventID)
	}
}

func (t *CalendarTool) deleteSingleInstance(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, eventID string) (string, error) {
	// Guard: if the event ID is a recurring master, deleting it would remove the
	// entire series. Fetch the event to check.
	raw, err := GetRawEvent(ctx, ts, cal.CalendarID, eventID)
	if err != nil {
		return "", err
	}
	if len(raw.Recurrence) > 0 {
		return "This is a recurring event series. To delete a single occurrence, first list events " +
			"to get the specific instance ID, then delete that instance. " +
			"To delete all occurrences, use scope='all'. " +
			"To delete this and all future occurrences, use scope='this_and_future'.", nil
	}

	if err := DeleteEvent(ctx, ts, cal.CalendarID, eventID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Event deleted successfully. [ID: %s]", eventID), nil
}

func (t *CalendarTool) deleteAllInstances(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, eventID string) (string, error) {
	masterID := masterEventID(eventID)
	if err := DeleteEvent(ctx, ts, cal.CalendarID, masterID); err != nil {
		return "", err
	}
	return fmt.Sprintf("All instances of recurring event deleted. [ID: %s]", masterID), nil
}

func (t *CalendarTool) deleteThisAndFuture(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, eventID string) (string, error) {
	// Get the instance to find its start time
	instance, err := GetRawEvent(ctx, ts, cal.CalendarID, eventID)
	if err != nil {
		return "", fmt.Errorf("fetching event instance: %w", err)
	}

	masterID := masterEventID(eventID)
	if masterID == eventID && instance.RecurringEventId == "" {
		// Not a recurring event instance, just delete the single event
		return t.deleteSingleInstance(ctx, ts, cal, eventID)
	}
	if instance.RecurringEventId != "" {
		masterID = instance.RecurringEventId
	}

	// Get the master event
	master, err := GetRawEvent(ctx, ts, cal.CalendarID, masterID)
	if err != nil {
		return "", fmt.Errorf("fetching master event: %w", err)
	}

	// Trim the RRULE with UNTIL set before this instance
	untilTime := instanceStartTime(instance)
	if untilTime.IsZero() {
		return "", fmt.Errorf("could not determine instance start time")
	}
	until := untilTime.Add(-time.Second).UTC().Format("20060102T150405Z")

	master.Recurrence = trimRecurrenceUntil(master.Recurrence, until)
	_, err = UpdateRawEvent(ctx, ts, cal.CalendarID, masterID, master)
	if err != nil {
		return "", fmt.Errorf("trimming recurring series: %w", err)
	}

	return fmt.Sprintf("This and all future instances deleted. Series trimmed at %s. [ID: %s]",
		untilTime.Format("2006-01-02"), masterID), nil
}

// resolveEventID resolves an event ID from input, either directly or by title search.
// Returns (eventID, "", nil) on success, ("", message, nil) for user-facing guidance,
// or ("", "", error) on actual errors.
func (t *CalendarTool) resolveEventID(ctx context.Context, ts oauth2.TokenSource, cal *TopicCalendar, input map[string]any) (string, string, error) {
	eventID, _ := input["event_id"].(string)
	if eventID != "" {
		return eventID, "", nil
	}

	title, _ := input["title"].(string)
	if title == "" {
		return "", "", nil
	}

	matches, err := FindEventByTitle(ctx, ts, cal.CalendarID, title)
	if err != nil {
		return "", "", err
	}
	if len(matches) == 0 {
		return "", fmt.Sprintf("No event found matching '%s'.", title), nil
	}
	if len(matches) > 1 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Multiple events match '%s'. Please specify event_id:\n", title))
		for _, m := range matches {
			sb.WriteString(fmt.Sprintf("- %s (%s) [ID: %s]\n", m.Title, formatEventTime(m.Start), m.ID))
		}
		return "", sb.String(), nil
	}
	return matches[0].ID, "", nil
}

func extractScope(input map[string]any) string {
	scope, _ := input["scope"].(string)
	switch scope {
	case "all", "this_and_future":
		return scope
	default:
		return "single"
	}
}

func buildUpdates(input map[string]any) map[string]any {
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
	return updates
}

func applyUpdatesToRaw(raw *gcalendar.Event, updates map[string]any, timezone string) {
	if title, ok := updates["title"].(string); ok && title != "" {
		raw.Summary = title
	}
	if desc, ok := updates["description"].(string); ok {
		raw.Description = desc
	}
	if loc, ok := updates["location"].(string); ok {
		raw.Location = loc
	}
	if startStr, ok := updates["start"].(string); ok && startStr != "" {
		t, err := parseFlexibleTime(startStr, timezone)
		if err == nil {
			raw.Start = &gcalendar.EventDateTime{
				DateTime: t.Format(time.RFC3339),
				TimeZone: timezone,
			}
		}
	}
	if endStr, ok := updates["end"].(string); ok && endStr != "" {
		t, err := parseFlexibleTime(endStr, timezone)
		if err == nil {
			raw.End = &gcalendar.EventDateTime{
				DateTime: t.Format(time.RFC3339),
				TimeZone: timezone,
			}
		}
	}
}

func validateRRULE(rule string) error {
	if !strings.HasPrefix(rule, "RRULE:") {
		return fmt.Errorf("recurrence rule must start with RRULE: prefix (e.g., RRULE:FREQ=WEEKLY;BYDAY=TU)")
	}
	body := strings.TrimPrefix(rule, "RRULE:")
	if !strings.Contains(body, "FREQ=") {
		return fmt.Errorf("recurrence rule must contain FREQ= (e.g., RRULE:FREQ=WEEKLY;BYDAY=TU)")
	}
	freqValues := map[string]bool{"DAILY": true, "WEEKLY": true, "MONTHLY": true, "YEARLY": true}
	for _, kv := range strings.Split(body, ";") {
		if strings.HasPrefix(kv, "FREQ=") {
			freq := strings.TrimPrefix(kv, "FREQ=")
			if !freqValues[freq] {
				return fmt.Errorf("FREQ must be one of: DAILY, WEEKLY, MONTHLY, YEARLY")
			}
			return nil
		}
	}
	return fmt.Errorf("recurrence rule must contain a valid FREQ value")
}

// instanceStartTime extracts the start time from a Google Calendar event instance.
func instanceStartTime(e *gcalendar.Event) time.Time {
	if e.OriginalStartTime != nil {
		if e.OriginalStartTime.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, e.OriginalStartTime.DateTime); err == nil {
				return t
			}
		}
		if e.OriginalStartTime.Date != "" {
			if t, err := time.Parse("2006-01-02", e.OriginalStartTime.Date); err == nil {
				return t
			}
		}
	}
	if e.Start != nil {
		if e.Start.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, e.Start.DateTime); err == nil {
				return t
			}
		}
		if e.Start.Date != "" {
			if t, err := time.Parse("2006-01-02", e.Start.Date); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// instanceEndTime extracts the end time from a Google Calendar event instance.
func instanceEndTime(e *gcalendar.Event) time.Time {
	if e.End != nil {
		if e.End.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, e.End.DateTime); err == nil {
				return t
			}
		}
		if e.End.Date != "" {
			if t, err := time.Parse("2006-01-02", e.End.Date); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// trimRecurrenceUntil modifies RRULE strings to add or replace an UNTIL clause.
func trimRecurrenceUntil(recurrence []string, until string) []string {
	var result []string
	for _, rule := range recurrence {
		if !strings.HasPrefix(rule, "RRULE:") {
			result = append(result, rule)
			continue
		}
		body := strings.TrimPrefix(rule, "RRULE:")
		var parts []string
		hasUntil := false
		for _, part := range strings.Split(body, ";") {
			if strings.HasPrefix(part, "UNTIL=") {
				parts = append(parts, "UNTIL="+until)
				hasUntil = true
			} else if strings.HasPrefix(part, "COUNT=") {
				// Remove COUNT when adding UNTIL (they are mutually exclusive)
				continue
			} else {
				parts = append(parts, part)
			}
		}
		if !hasUntil {
			parts = append(parts, "UNTIL="+until)
		}
		result = append(result, "RRULE:"+strings.Join(parts, ";"))
	}
	return result
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
