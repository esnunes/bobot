---
title: "Show full event details in calendar list output"
type: feat
date: 2026-03-09
issue: https://github.com/esnunes/bobot/issues/50
---

# Show Full Event Details in Calendar List Output

## Overview

The calendar tool's `list` command only shows event title, time, location, and ID. The `EventInfo` struct already fetches description, location, and HTML link from the Google Calendar API — but `execList()` doesn't include them in the output. Additionally, location may not be appearing despite code that checks for it.

## Proposed Solution

Update the `execList()` output formatting in `tools/calendar/calendar.go` to include description, Google Calendar link, and fix location display. All three fields are already available in the `EventInfo` struct — this is purely an output formatting change.

## Acceptance Criteria

- [x] Event description is displayed in full (not truncated) when present
- [x] Google Calendar HTML link is displayed when present
- [x] Location is reliably displayed when present on an event
- [x] Empty fields are not shown (no empty labels)
- [x] Existing output format (title, time, ID) is preserved

## Implementation

### 1. Investigate location bug

**File:** `tools/calendar/client.go` — `eventToInfo()` function

The current code maps `Location: e.Location` directly from the Google API response. The `execList()` code checks `e.Location != ""` and appends it. Verify:
- Is the Google Calendar API actually returning location data? (Add temporary logging or inspect API response)
- Is there a field mapping issue in `eventToInfo()`?
- Are test events actually populated with location in Google Calendar?

If location is correctly mapped but events simply lack location data, no code fix is needed — just confirm it works when location exists.

### 2. Update `execList()` output formatting

**File:** `tools/calendar/calendar.go` — `execList()` method (lines ~211-228)

Current format per event:
```
- **Meeting Title** (Mon Mar 9, 2:30 PM - Mon Mar 9, 3:30 PM) @ Location [ID: abc123]
```

Updated format per event (all optional fields shown only when non-empty):
```
- **Meeting Title** (Mon Mar 9, 2:30 PM - Mon Mar 9, 3:30 PM) [ID: abc123]
  Location: Conference Room A
  Description: Weekly team sync to discuss project status
  Link: https://calendar.google.com/calendar/event?eid=abc123
```

Changes to the formatting loop in `execList()`:
- Keep the first line as-is (title + time + ID)
- After the first line, conditionally append indented lines for Location, Description, and Link
- Only show each field when its value is non-empty
- Add a blank line between events for readability when details are present

### 3. Test

- Test with events that have all three fields populated
- Test with events missing some/all optional fields (no empty labels should appear)
- Test with all-day events
- Verify the output reads well for the LLM (since this is tool output consumed by the assistant)

## References

- `tools/calendar/calendar.go:211-228` — current `execList()` formatting
- `tools/calendar/client.go:26-35` — `EventInfo` struct (already has Description, HTMLLink)
- `tools/calendar/client.go:220-246` — `eventToInfo()` mapping
