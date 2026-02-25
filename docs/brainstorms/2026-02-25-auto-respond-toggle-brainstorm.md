# Auto-Respond Toggle Per Topic

**Date:** 2026-02-25
**Status:** Draft
**Issue:** esnunes/bobot#30

## What We're Building

A UI toggle in the Topic Settings page that allows topic owners and admins to enable/disable `auto_respond` per topic. When enabled, the bot responds to every message. When disabled, the bot only responds when `@bobot` is mentioned.

The backend already fully supports this feature (`topics.auto_respond` column + `db.SetTopicAutoRespond()`). This is purely a UI addition.

## Why This Approach

The simplest path: add a toggle row to the existing "Topic Settings" section on the settings page, matching the established pattern of mute/auto-read toggles. No new pages, no new components needed.

## Key Decisions

1. **Permissions: Owner + Admins** - Only the topic owner or admin-role users can toggle auto-respond. This is a topic-wide setting affecting all members, so it should be restricted.

2. **Visibility: Hidden for non-privileged members** - Non-owner/non-admin members won't see the toggle at all (simpler UI).

3. **Real-time updates: On next page load** - When auto-respond is toggled, other connected members see the change (e.g., `@bobot` button appearing/disappearing in chat footer) only when they navigate back to the chat. No WebSocket broadcast needed.

4. **Bobot topic: Hide toggle** - The 1:1 "bobot" topic (auto-created per user) should always have `auto_respond=true`. The toggle is hidden for this topic to prevent users from accidentally silencing their personal bot chat.

## Scope

### In Scope
- New toggle row in settings.html "Topic Settings" section
- New HTTP handler for toggling auto-respond (POST/DELETE pattern like mute/auto-read)
- Server-side permission check (owner or admin)
- Conditional rendering: hide toggle for bobot topic, hide for non-owner/non-admin
- Pass `auto_respond` and `is_bobot_topic` data to the settings template
- JS handler in settings.js for the toggle interaction

### Out of Scope
- Real-time WebSocket broadcast of setting changes
- Changing the auto_respond behavior itself (already working)
- Any changes to the chat page (already handles auto_respond correctly)
