---
date: 2026-02-24
topic: settings-page-redesign
issue: https://github.com/esnunes/bobot/issues/28
---

# Settings Page Redesign

## What We're Building

Replace the current slide-over menu overlay in the topic chat view with a dedicated, full settings page. The page uses collapsible `<details>` sections (all open by default) to organize settings into clear groups with descriptive labels.

The settings page is **composable**: when accessed from a topic chat, it shows topic-specific sections plus a global section. When accessed from the chats list, it shows only the global section. This eliminates the confusion about which settings are topic-specific vs. global.

Additionally, the hamburger menu icon in the topic chat header will be replaced with a settings icon to better communicate the page's purpose.

## Why This Approach

**Approaches considered:**

1. **Full settings page with collapsible sections** (chosen) — Provides ample room for descriptions, clear grouping, and scales well with future settings. Uses the existing `<details>/<summary>` pattern already present in admin pages.

2. **Improved slide-over menu** — Quick fix but limited by the 240px-wide panel. Adding descriptions would make it even more scrollable. Doesn't fundamentally solve the space problem.

3. **Bottom sheet (mobile pattern)** — More native mobile feel, but introduces a new UI pattern not used elsewhere in the app and adds implementation complexity.

The full settings page was chosen because it solves all three problems identified in the issue (clutter, unclear context, poor discoverability) while using existing patterns and being naturally mobile-friendly as a single scrollable page.

## Key Decisions

- **Full page navigation, not an overlay**: The settings icon in the header navigates to a dedicated page (via HTMX body swap, consistent with existing navigation). Trade-off: one more tap to return vs. much more space and clarity.

- **Collapsible `<details>` sections, all open by default**: Users see everything at a glance but can collapse sections they don't need. Can re-evaluate defaults later based on usage.

- **Composable layout (topic + global)**: The global section (account, profile, admin, logout) appears on all settings pages. Topic-specific sections (details, settings, tools, danger zone) are added when there's a topic context. This ensures global settings are always reachable from any settings page.

- **Helper descriptions on all toggle settings**: Each setting gets a short one-line description explaining what it does (e.g., "Auto-read — Automatically mark messages as read when you open the topic"). Addresses the issue's complaint about settings being unclear.

- **Settings icon replaces hamburger**: The header button changes from a hamburger icon to a settings/gear icon to communicate the page's purpose.

- **Notifications toggle is global, Mute is per-topic**: "Enable push notifications" is a browser/device-level setting and belongs in the global Account section. "Mute topic" silences a specific topic and stays in Topic Settings. These remain separate controls with clear descriptions.

- **Editable user profile in Account section**: The Account section includes editable display name. Each section uses HTMX for independent save-per-group, so users can update their profile without affecting other settings.

- **Inline previews for Skills and Schedules**: Instead of plain navigation links, show a compact list of skill/schedule names within the Topic Tools section, with a "Manage" link to the full page.

## Page Structure

### Topic Settings Page (from topic chat)

```
[Back arrow]  [Topic Name]  [—]     ← header (back returns to chat)

▾ Topic Details
  Members: Alice, Bob, Charlie
  (expandable member list with display names)

▾ Topic Settings
  Mute                   [toggle]
  Silence push notifications for this topic

  Auto-read              [toggle]
  Automatically mark messages as read when you open this topic

▾ Topic Tools
  Skills:
    - Weather lookup
    - Translation
    [Manage Skills →]
  Schedules:
    - Daily standup (9am)
    [Manage Schedules →]

▾ Danger Zone
  [Delete Topic] / [Leave Topic]

─────────────────────────────────

▾ Account
  Display name: [Eduardo     ] [Save]

  Push Notifications     [toggle]
  Enable push notifications for this device

  Admin →                (admin only)
  [Logout]
```

### Chats List Settings Page (from chats list)

```
[Back arrow]  [Settings]  [—]      ← header (back returns to chats)

▾ Account
  Display name: [Eduardo     ] [Save]

  Push Notifications     [toggle]
  Enable push notifications for this device

  Admin →                (admin only)
  [Logout]
```

## Resolved Questions

- **User profile in Account section?** Yes — include editable display name with HTMX save-per-group.
- **Consolidate Notifications and Mute?** No — keep separate. "Push Notifications" is a global device setting (Account section). "Mute" is per-topic (Topic Settings section).
- **Skills/Schedules display?** Inline preview showing compact list of names with a "Manage" link to the full page.

## Next Steps

-> `/workflows:plan` for implementation details
