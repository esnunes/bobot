---
title: "Calendar list command missing description, link, and location in output"
date: 2026-03-09
category: logic-errors
module: calendar
tags:
  - formatting
  - calendar
  - google-calendar
  - tool-output
severity: low
time_to_resolve: 15m
---

# Calendar List Missing Event Details

## Problem

The calendar tool's `execList()` function produced a single-line output per event that omitted two fields entirely — **Description** and **HTMLLink** (the Google Calendar link) — and displayed **Location** inline with an `@ ` prefix on the same line as the event title and time.

Even though the `EventInfo` struct (`client.go:26-35`) carried all three fields and `eventToInfo()` (`client.go:220-246`) populated them from the Google Calendar API, the list formatting code never rendered `Description` or `HTMLLink`.

**Symptoms:**
- Events listed with no description text, even when the Google Calendar event had one
- No Google Calendar link in the output
- Location shown inline as `@ Some Place`, easily missed with long location strings

## Root Cause

The fields were correctly fetched from the Google Calendar API and stored in `EventInfo`. The problem was purely in the **display/formatting layer** inside `execList()`. There were no conditional blocks for `e.Description` or `e.HTMLLink`, so those fields were silently discarded during output generation. The data was available; it was never rendered.

## Solution

Updated the formatting loop in `execList()` (`calendar.go:215-233`) to add indented detail lines for Location, Description, and Link — each shown only when non-empty.

**Before:**

```go
if e.Location != "" {
    sb.WriteString(fmt.Sprintf(" @ %s", e.Location))
}
sb.WriteString(fmt.Sprintf(" [ID: %s]", e.ID))
sb.WriteString("\n")
```

**After:**

```go
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
```

## Investigation Notes

Location handling was **not a bug** — the original code correctly checked `e.Location != ""` and rendered it when present. It used an inline format (`@ Place`) rather than a dedicated detail line. The change was a formatting improvement for consistency, not a correctness fix.

## Prevention

**If you fetch it and store it, you must render it — unused struct fields are silent bugs.**

When implementing output formatting for a struct:
1. Compare every populated struct field against the output function — each field should either appear in the output or have an explicit reason for omission
2. Write output-driven tests that assert the formatted string contains values for all non-empty fields
3. Trace API-to-output as a single pipeline: Fetch → Store → Render. If steps 1-2 include fields that step 3 does not, that's a gap

The indented `Key: Value` detail-line pattern used here is consistent with other tools in the codebase (Spotify `execPlaybackStatus`, web search, cron).

## References

- **Issue:** [#50 - Show full event details in calendar list output](https://github.com/esnunes/bobot/issues/50)
- **PR:** [#51 - feat(calendar): show full event details in list output](https://github.com/esnunes/bobot/pull/51)
- **Plan:** `docs/plans/2026-03-09-feat-calendar-full-event-details-plan.md`
- **Original calendar plan:** `docs/plans/2026-02-26-feat-google-calendar-integration-plan.md`
- **Similar pattern:** `tools/spotify/spotify.go` — `execPlaybackStatus()` uses the same indented detail-line format
