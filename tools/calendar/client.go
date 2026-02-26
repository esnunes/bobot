// tools/calendar/client.go
package calendar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/oauth2"
	gcalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// CalendarInfo represents a Google Calendar for the picker UI.
type CalendarInfo struct {
	ID       string
	Name     string
	Timezone string
	Primary  bool
}

// EventInfo represents a Google Calendar event.
type EventInfo struct {
	ID          string
	Title       string
	Description string
	Location    string
	Start       string
	End         string
	AllDay      bool
	HTMLLink    string
}

// ListCalendars fetches the user's calendar list.
func ListCalendars(ctx context.Context, ts oauth2.TokenSource) ([]CalendarInfo, error) {
	srv, err := gcalendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("creating calendar service: %w", err)
	}

	list, err := srv.CalendarList.List().ShowHidden(false).Do()
	if err != nil {
		return nil, mapAPIError(err)
	}

	var calendars []CalendarInfo
	for _, entry := range list.Items {
		calendars = append(calendars, CalendarInfo{
			ID:       entry.Id,
			Name:     entry.Summary,
			Timezone: entry.TimeZone,
			Primary:  entry.Primary,
		})
	}
	return calendars, nil
}

// ListEvents lists events in the given calendar within a time range.
func ListEvents(ctx context.Context, ts oauth2.TokenSource, calendarID string, timeMin, timeMax time.Time, timezone string) ([]EventInfo, error) {
	srv, err := gcalendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("creating calendar service: %w", err)
	}

	call := srv.Events.List(calendarID).
		ShowDeleted(false).
		SingleEvents(true).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		OrderBy("startTime").
		MaxResults(25)

	if timezone != "" {
		call = call.TimeZone(timezone)
	}

	events, err := call.Do()
	if err != nil {
		return nil, mapAPIError(err)
	}

	var result []EventInfo
	for _, item := range events.Items {
		result = append(result, eventToInfo(item))
	}
	return result, nil
}

// CreateEvent creates a new event in the calendar.
func CreateEvent(ctx context.Context, ts oauth2.TokenSource, calendarID string, title, description, location string, start, end time.Time, allDay bool, timezone string) (*EventInfo, error) {
	srv, err := gcalendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("creating calendar service: %w", err)
	}

	event := &gcalendar.Event{
		Summary:     title,
		Description: description,
		Location:    location,
	}

	if allDay {
		event.Start = &gcalendar.EventDateTime{Date: start.Format("2006-01-02")}
		event.End = &gcalendar.EventDateTime{Date: end.Add(24 * time.Hour).Format("2006-01-02")}
	} else {
		event.Start = &gcalendar.EventDateTime{
			DateTime: start.Format(time.RFC3339),
			TimeZone: timezone,
		}
		event.End = &gcalendar.EventDateTime{
			DateTime: end.Format(time.RFC3339),
			TimeZone: timezone,
		}
	}

	created, err := srv.Events.Insert(calendarID, event).SendUpdates("none").Do()
	if err != nil {
		return nil, mapAPIError(err)
	}

	info := eventToInfo(created)
	return &info, nil
}

// UpdateEvent patches an existing event.
func UpdateEvent(ctx context.Context, ts oauth2.TokenSource, calendarID, eventID string, updates map[string]any, timezone string) (*EventInfo, error) {
	srv, err := gcalendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("creating calendar service: %w", err)
	}

	patch := &gcalendar.Event{}

	if title, ok := updates["title"].(string); ok && title != "" {
		patch.Summary = title
	}
	if desc, ok := updates["description"].(string); ok {
		patch.Description = desc
	}
	if loc, ok := updates["location"].(string); ok {
		patch.Location = loc
	}
	if startStr, ok := updates["start"].(string); ok && startStr != "" {
		t, err := parseFlexibleTime(startStr, timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid start time: %w", err)
		}
		patch.Start = &gcalendar.EventDateTime{
			DateTime: t.Format(time.RFC3339),
			TimeZone: timezone,
		}
	}
	if endStr, ok := updates["end"].(string); ok && endStr != "" {
		t, err := parseFlexibleTime(endStr, timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid end time: %w", err)
		}
		patch.End = &gcalendar.EventDateTime{
			DateTime: t.Format(time.RFC3339),
			TimeZone: timezone,
		}
	}

	patched, err := srv.Events.Patch(calendarID, eventID, patch).SendUpdates("none").Do()
	if err != nil {
		return nil, mapAPIError(err)
	}

	info := eventToInfo(patched)
	return &info, nil
}

// DeleteEvent deletes an event from the calendar.
func DeleteEvent(ctx context.Context, ts oauth2.TokenSource, calendarID, eventID string) error {
	srv, err := gcalendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("creating calendar service: %w", err)
	}

	if err := srv.Events.Delete(calendarID, eventID).SendUpdates("none").Do(); err != nil {
		return mapAPIError(err)
	}
	return nil
}

// FindEventByTitle searches for events matching a title (case-insensitive).
func FindEventByTitle(ctx context.Context, ts oauth2.TokenSource, calendarID, title string) ([]EventInfo, error) {
	srv, err := gcalendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("creating calendar service: %w", err)
	}

	// Search upcoming events (next 30 days)
	now := time.Now()
	events, err := srv.Events.List(calendarID).
		ShowDeleted(false).
		SingleEvents(true).
		TimeMin(now.Format(time.RFC3339)).
		TimeMax(now.AddDate(0, 1, 0).Format(time.RFC3339)).
		Q(title).
		OrderBy("startTime").
		MaxResults(10).
		Do()
	if err != nil {
		return nil, mapAPIError(err)
	}

	var result []EventInfo
	for _, item := range events.Items {
		if strings.Contains(strings.ToLower(item.Summary), strings.ToLower(title)) {
			result = append(result, eventToInfo(item))
		}
	}
	return result, nil
}

func eventToInfo(e *gcalendar.Event) EventInfo {
	info := EventInfo{
		ID:          e.Id,
		Title:       e.Summary,
		Description: e.Description,
		Location:    e.Location,
		HTMLLink:    e.HtmlLink,
	}

	if e.Start != nil {
		if e.Start.Date != "" {
			info.Start = e.Start.Date
			info.AllDay = true
		} else {
			info.Start = e.Start.DateTime
		}
	}
	if e.End != nil {
		if e.End.Date != "" {
			info.End = e.End.Date
		} else {
			info.End = e.End.DateTime
		}
	}

	return info
}

func parseFlexibleTime(s, timezone string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try date-only
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}

	// Try datetime without timezone
	loc := time.UTC
	if timezone != "" {
		if l, err := time.LoadLocation(timezone); err == nil {
			loc = l
		}
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, loc); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unrecognized time format: %s", s)
}

func mapAPIError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return err
	}

	switch apiErr.Code {
	case 401:
		return fmt.Errorf("Google Calendar access expired or was revoked. The topic owner needs to reconnect from settings.")
	case 404:
		return fmt.Errorf("Event not found.")
	case 429:
		return fmt.Errorf("Google Calendar is busy, try again in a moment.")
	case 403:
		for _, e := range apiErr.Errors {
			if e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded" {
				return fmt.Errorf("Google Calendar is busy, try again in a moment.")
			}
		}
		return fmt.Errorf("Insufficient permissions for this calendar operation.")
	}

	if apiErr.Code >= 500 {
		return fmt.Errorf("Google Calendar is temporarily unavailable.")
	}

	return err
}

