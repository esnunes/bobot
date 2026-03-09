---
title: "feat: Add recurring event support to the calendar tool"
type: feat
date: 2026-03-09
issue: https://github.com/esnunes/bobot/issues/52
---

# feat: Add recurring event support to the calendar tool

## Overview

Extend the calendar tool to support creating, updating, and deleting recurring events via Google Calendar API. The LLM translates natural language recurrence descriptions (e.g., "every Tuesday") into iCal RRULE strings. Update and delete operations support three scopes: single instance, all instances, and this-and-future. Listing continues to work as-is since `SingleEvents(true)` already expands recurring events.

## Problem Statement / Motivation

The calendar tool only supports one-time events. Users cannot create events that repeat on a schedule (weekly standups, monthly reviews, daily reminders). This forces manual creation of each occurrence, defeating the purpose of an AI assistant managing the calendar. Recurring events already display correctly in list output (Google Calendar expands them), but there is no way to create, update, or delete them with recurrence awareness.

## Proposed Solution

Add a `recurrence` parameter to the create command and a `scope` parameter to update/delete commands. Extend `EventInfo` with recurrence metadata so the LLM can distinguish recurring from non-recurring events. The tool handles master-vs-instance ID resolution internally, so the LLM only needs to pass event IDs from list results.

## Technical Approach

### Key Design Decisions

1. **Default scope is `single`** -- least destructive option when the LLM or user doesn't specify scope
2. **Tool derives master ID from instance ID internally** -- the LLM passes whatever event ID it has; the tool strips `_{timestamp}` suffix when `scope=all`
3. **Basic RRULE validation server-side** -- verify `RRULE:` prefix and `FREQ=` presence; let Google API handle detailed validation; map 400 errors to user-friendly messages
4. **Accept single RRULE string** -- wrap as `[]string{recurrence}` for Google API internally; EXDATE/RDATE are managed through scope operations, not user input
5. **Rollback for this-and-future** -- if step 2 (create new series) fails, attempt to restore original RRULE on master; log if restore also fails
6. **LLM should ask for clarification on ambiguous scope** -- documented in tool description

### Architecture

No new packages or databases. All changes are within `tools/calendar/`:

```
tools/calendar/
  calendar.go  -- Schema(), Description(), ParseArgs(), Execute(), exec* handlers
  client.go    -- EventInfo struct, CreateEvent, UpdateEvent, DeleteEvent, eventToInfo, new helpers
```

### Implementation Phases

#### Phase 1: EventInfo and client.go updates

Extend the data model and API client to support recurrence.

**`client.go` -- EventInfo struct** (line 26):

```go
type EventInfo struct {
	ID               string
	Title            string
	Description      string
	Location         string
	Start            string
	End              string
	AllDay           bool
	HTMLLink         string
	// New fields for recurrence
	IsRecurring      bool
	RecurringEventID string   // master event ID (populated on instances)
	Recurrence       []string // RRULE strings (populated on master events)
}
```

**`client.go` -- Update `eventToInfo`** (line 220):

Add extraction of recurrence fields from Google API response:

```go
func eventToInfo(e *gcalendar.Event) EventInfo {
	info := EventInfo{
		// ... existing fields ...
	}
	// Recurrence metadata
	if len(e.Recurrence) > 0 {
		info.IsRecurring = true
		info.Recurrence = e.Recurrence
	}
	if e.RecurringEventId != "" {
		info.IsRecurring = true
		info.RecurringEventID = e.RecurringEventId
	}
	return info
}
```

**`client.go` -- Update `CreateEvent` signature**:

Add `recurrence []string` parameter. Set `event.Recurrence` before insert.

```go
func CreateEvent(ctx context.Context, ts oauth2.TokenSource, calendarID string,
	title, description, location string, start, end time.Time,
	allDay bool, timezone string, recurrence []string) (*EventInfo, error)
```

**`client.go` -- New `GetEvent` function**:

Needed for this-and-future operations to fetch the master event:

```go
func GetEvent(ctx context.Context, ts oauth2.TokenSource, calendarID, eventID string) (*EventInfo, error)
```

**`client.go` -- New `masterEventID` helper**:

Derive master event ID from an instance ID:

```go
func masterEventID(eventID string) string {
	// Instance IDs: "{masterId}_{YYYYMMDDTHHMMSSZ}" or "{masterId}_{YYYYMMDD}"
	// Master IDs use base32hex: [a-v0-9]{5,1024}
	if idx := strings.LastIndex(eventID, "_"); idx > 0 {
		suffix := eventID[idx+1:]
		// Check if suffix looks like a timestamp (8+ digits/T/Z chars)
		if len(suffix) >= 8 && (suffix[0] >= '0' && suffix[0] <= '9') {
			return eventID[:idx]
		}
	}
	return eventID // already a master ID
}
```

**`client.go` -- Add 400 error handling in `mapAPIError`** (line 276):

```go
case 400:
	return fmt.Errorf("Invalid request. If creating a recurring event, check the recurrence rule format (e.g., RRULE:FREQ=WEEKLY;BYDAY=TU).")
```

**`client.go` -- Update `FindEventByTitle` to deduplicate recurring instances**:

When multiple results share the same `RecurringEventId`, group them and return only the first instance (with the master ID available via `RecurringEventID` field).

#### Phase 2: Schema, Description, and ParseArgs updates

**`calendar.go` -- Update `Schema()`** (line 33):

Add two new properties:

```go
"recurrence": map[string]any{
	"type": "string",
	"description": "iCal RRULE string for recurring events (optional). " +
		"Examples: RRULE:FREQ=DAILY, RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR, " +
		"RRULE:FREQ=MONTHLY;BYMONTHDAY=1, RRULE:FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=9, " +
		"RRULE:FREQ=WEEKLY;BYDAY=TU;COUNT=10, RRULE:FREQ=DAILY;UNTIL=20261231T235959Z. " +
		"Must start with RRULE: prefix. Used only with create command.",
},
"scope": map[string]any{
	"type": "string",
	"enum": []string{"single", "all", "this_and_future"},
	"description": "Scope for update/delete on recurring events. " +
		"'single': modify only this instance (default). " +
		"'all': modify the entire recurring series. " +
		"'this_and_future': modify this instance and all future instances. " +
		"When the user's intent is ambiguous for a recurring event, ask them to clarify. " +
		"Ignored for non-recurring events.",
},
```

**`calendar.go` -- Update `Description()`** (line 29):

```go
func (t *CalendarTool) Description() string {
	return "Manage Google Calendar events for this topic. Supports one-time and recurring events. " +
		"Use the recurrence parameter with create to set up repeating events (pass an iCal RRULE string). " +
		"For recurring events, use the scope parameter with update/delete to choose: " +
		"'single' (this instance only), 'all' (entire series), or 'this_and_future' (this and all later instances). " +
		"When a user's intent about recurring event scope is ambiguous, ask them to clarify. " +
		"Event IDs from list results can be used directly -- the tool handles instance-vs-series resolution internally."
}
```

**`calendar.go` -- Update `ParseArgs`**:

- **create**: Add `--recurrence=` flag. Use `strings.SplitN(p, "=", 2)` for all flag parsing to handle `=` chars in RRULE values.
- **update**: Add `--scope=` and `--recurrence=` flags.
- **delete**: Accept optional `--scope=` flag after event_id.

```go
// In create ParseArgs, after existing positional args:
for _, p := range parts[4:] {
	if strings.HasPrefix(p, "--recurrence=") {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			result["recurrence"] = kv[1]
		}
	} else if strings.HasPrefix(p, "--") {
		// skip unknown flags
	} else {
		// accumulate description words
	}
}
```

#### Phase 3: Execute handler updates

**`calendar.go` -- Update `execCreate`**:

Extract `recurrence` from input, validate basic format, pass to `CreateEvent`:

```go
var recurrence []string
if rec, ok := input["recurrence"].(string); ok && rec != "" {
	if err := validateRRULE(rec); err != nil {
		return "", err
	}
	recurrence = []string{rec}
}

event, err := CreateEvent(ctx, ts, cal.CalendarID, title, description, location,
	startTime, endTime, allDay, cal.Timezone, recurrence)
```

Add recurrence info to create confirmation output:

```go
if len(recurrence) > 0 {
	sb.WriteString(fmt.Sprintf("Recurrence: %s\n", recurrence[0]))
}
```

**`calendar.go` -- Update `execUpdate`**:

Add scope-aware routing:

```go
func (t *CalendarTool) execUpdate(ctx context.Context, ts oauth2.TokenSource,
	cal *TopicCalendar, input map[string]any) (string, error) {

	eventID := resolveEventID(input) // existing ID/title resolution
	scope := extractScope(input)     // "single" (default), "all", "this_and_future"

	switch scope {
	case "all":
		masterID := masterEventID(eventID)
		return t.updateAllInstances(ctx, ts, cal, masterID, input)
	case "this_and_future":
		return t.updateThisAndFuture(ctx, ts, cal, eventID, input)
	default: // "single"
		return t.updateSingleInstance(ctx, ts, cal, eventID, input)
	}
}
```

- `updateSingleInstance`: Existing `UpdateEvent` logic (Patch with instance ID).
- `updateAllInstances`: Patch with master ID. Also handle recurrence changes.
- `updateThisAndFuture`: (1) Fetch master event, (2) save original RRULE, (3) trim master with UNTIL before target instance, (4) create new series from target instance with changes, (5) rollback master if create fails.

**`calendar.go` -- Update `execDelete`**:

Add scope-aware routing:

```go
func (t *CalendarTool) execDelete(ctx context.Context, ts oauth2.TokenSource,
	cal *TopicCalendar, input map[string]any) (string, error) {

	eventID := resolveEventID(input)
	scope := extractScope(input)

	switch scope {
	case "all":
		masterID := masterEventID(eventID)
		return t.deleteAllInstances(ctx, ts, cal, masterID)
	case "this_and_future":
		return t.deleteThisAndFuture(ctx, ts, cal, eventID)
	default: // "single"
		return t.deleteSingleInstance(ctx, ts, cal, eventID)
	}
}
```

- `deleteSingleInstance`: Existing `DeleteEvent` logic.
- `deleteAllInstances`: Delete with master ID.
- `deleteThisAndFuture`: Fetch master, trim RRULE with UNTIL before target instance's `OriginalStartTime`.

**`calendar.go` -- Update `execList`**:

Add `(recurring)` marker and recurrence info to output:

```go
if e.IsRecurring {
	sb.WriteString(" (recurring)")
}
```

#### Phase 4: RRULE validation helper

Simple validation -- no external library needed:

```go
func validateRRULE(rule string) error {
	if !strings.HasPrefix(rule, "RRULE:") {
		return fmt.Errorf("recurrence rule must start with RRULE: prefix (e.g., RRULE:FREQ=WEEKLY;BYDAY=TU)")
	}
	parts := strings.TrimPrefix(rule, "RRULE:")
	if !strings.Contains(parts, "FREQ=") {
		return fmt.Errorf("recurrence rule must contain FREQ= (e.g., RRULE:FREQ=WEEKLY;BYDAY=TU)")
	}
	freqValues := []string{"DAILY", "WEEKLY", "MONTHLY", "YEARLY"}
	hasValidFreq := false
	for _, kv := range strings.Split(parts, ";") {
		if strings.HasPrefix(kv, "FREQ=") {
			freq := strings.TrimPrefix(kv, "FREQ=")
			for _, valid := range freqValues {
				if freq == valid {
					hasValidFreq = true
				}
			}
		}
	}
	if !hasValidFreq {
		return fmt.Errorf("FREQ must be one of: DAILY, WEEKLY, MONTHLY, YEARLY")
	}
	return nil
}
```

### Files Changed

| File | Changes |
|------|---------|
| `tools/calendar/client.go` | `EventInfo` struct (3 new fields), `eventToInfo`, `CreateEvent` signature, new `GetEvent`, `masterEventID` helper, `mapAPIError` (400 case), `FindEventByTitle` dedup |
| `tools/calendar/calendar.go` | `Schema()` (2 new properties), `Description()`, `ParseArgs` (recurrence/scope flags), `execCreate` (recurrence param), `execUpdate` (scope routing + 3 sub-handlers), `execDelete` (scope routing + 3 sub-handlers), `execList` (recurring marker), `validateRRULE` helper |

## Acceptance Criteria

### Functional Requirements

- [x] **Create**: `/calendar create` and LLM tool call accept `recurrence` parameter with RRULE string
- [x] **Create**: Recurring event is created as a single Google Calendar recurring event (not multiple individual events)
- [x] **Create**: Confirmation output includes the recurrence rule
- [x] **Update single**: Updating with `scope=single` (or no scope) modifies only the target instance
- [x] **Update all**: Updating with `scope=all` modifies the entire recurring series via master ID
- [x] **Update this-and-future**: Updating with `scope=this_and_future` trims the original series and creates a new one
- [x] **Update recurrence**: Can add/change/remove recurrence rule on an existing event
- [x] **Delete single**: Deleting with `scope=single` (or no scope) removes only the target instance
- [x] **Delete all**: Deleting with `scope=all` removes the entire recurring series
- [x] **Delete this-and-future**: Deleting with `scope=this_and_future` trims the series at the target instance
- [x] **List**: Recurring events display `(recurring)` marker in list output
- [x] **List**: No breaking changes to existing list behavior
- [x] **Schema**: Tool schema includes `recurrence` and `scope` properties with clear descriptions
- [x] **Description**: Tool description explains recurrence support and scope semantics
- [x] **Validation**: Invalid RRULE strings return a helpful error message
- [x] **Backward compat**: Non-recurring event operations are unaffected by the changes

### Error Handling

- [x] **Invalid RRULE**: Returns user-friendly error (not raw Google API error)
- [x] **Scope on non-recurring event**: Scope is silently ignored
- [x] **This-and-future rollback**: If new series creation fails, master RRULE is restored
- [x] **Google API 400**: Mapped to helpful error message about recurrence format

## Dependencies & Risks

**Dependencies**: None -- only uses existing Google Calendar API v3 library already in `go.mod`.

**Risks**:
- **This-and-future is not atomic**: Two API calls can partially fail. Mitigated by rollback strategy.
- **Instance ID parsing is heuristic**: The `masterEventID` helper uses format-based detection. If Google changes ID format, this could break. Mitigated by the pattern being stable since 2011.
- **LLM RRULE accuracy**: The LLM may generate incorrect RRULE strings for complex patterns. Mitigated by basic validation + Google API validation + examples in schema description.

## References & Research

### Internal References

- Calendar tool: `tools/calendar/calendar.go` (Schema, Execute, ParseArgs)
- Calendar client: `tools/calendar/client.go` (EventInfo, CreateEvent, UpdateEvent, DeleteEvent)
- Tool interface: `tools/registry.go` (Name, Description, Schema, ParseArgs, Execute, AdminOnly)
- Past solution: `docs/solutions/logic-errors/calendar-list-missing-event-details.md` (render pattern for new fields)
- Original calendar plan: `docs/plans/2026-02-26-feat-google-calendar-integration-plan.md`

### External References

- [Google Calendar API Recurring Events Guide](https://developers.google.com/workspace/calendar/api/guides/recurringevents)
- [Google Calendar API Events Resource](https://developers.google.com/workspace/calendar/api/v3/reference/events)
- [iCal RRULE RFC 5545](https://datatracker.ietf.org/doc/html/rfc5545#section-3.3.10)
- Go client: `google.golang.org/api/calendar/v3` (`Event.Recurrence []string`, `Event.RecurringEventId`)
