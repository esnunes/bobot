# User Differentiation in Group Topics

**Date:** 2026-02-27
**Status:** Draft
**Issue:** [#45](https://github.com/esnunes/bobot/issues/45) - Users are getting confused on topics with multiple members

## What We're Building

Visual differentiation of message senders in group topics so users can instantly identify who sent each message at a glance. Currently, all non-self messages appear as identical gray boxes with only a small 12px display name label.

**Solution:** Add Gravatar avatars with colored sender names per user, with WhatsApp-style message grouping for consecutive messages from the same sender.

## Why This Approach

**Client-side color assignment with Gravatar avatars** was chosen because:

- Color assignment for names is fully client-side - fits the existing JS-rendered message pattern
- Gravatar provides recognizable profile images without building an upload system
- Per-topic color assignment (based on member join order) ensures optimal color distribution within each conversation
- 6 bold colors keep visual distinction high while covering typical group sizes

**Backend changes are minimal** - only adding an email field to the User model and a settings UI for users to set their email address.

## Key Decisions

1. **Gravatar avatar + colored name** - Each user gets a Gravatar image (based on their email hash) and a matching colored sender name. Message bubble backgrounds stay neutral (current gray/lavender).

2. **Gravatar fallback** - When a user hasn't set an email or doesn't have a Gravatar, use Gravatar's built-in default image (generic gray silhouette).

3. **Email field on User model** - Users need a way to set their email address via the settings page to enable Gravatar. This is the only backend/DB change.

4. **Per-topic color assignment** - Colors assigned based on the member's position in the topic's member list, not derived from user ID. A user might be "green" in one topic and "blue" in another, but colors are optimally distributed within each conversation.

5. **6 bold colors** - A curated set of 6 highly distinct colors from the Catppuccin Latte palette. Colors repeat in groups larger than 6 (uncommon). Easy to tell apart at a glance.

6. **WhatsApp-style message grouping** - Consecutive messages from the same sender are visually grouped:
   - Sender name shown on the **first** message in a streak
   - Avatar shown on the **last** message in a streak
   - Each message still has its own bubble

7. **Bobot treated like any other participant** - The assistant gets a color from the same palette, no special visual treatment.

8. **Self messages unchanged** - The current right-aligned lavender style for the logged-in user stays as-is (no avatar/name needed since the user knows their own messages).

## Scope

**In scope:**
- Email field on User model (DB migration)
- Email input in settings page
- Gravatar URL generation (MD5 hash of lowercase trimmed email)
- Color palette definition (6 bold CSS tokens)
- Client-side color assignment logic per topic
- Avatar rendering (Gravatar image with gray default fallback)
- Colored sender name
- WhatsApp-style consecutive message grouping
- Works for both initial page load messages and real-time WebSocket messages

**Out of scope:**
- Profile picture upload system
- User-chosen colors or themes
- Changes to self-message styling
- Email verification

## Resolved Questions

1. **How many colors in the palette?** 6 bold, highly distinct colors from Catppuccin Latte. Enough for typical group sizes while ensuring each color is clearly different.

2. **Avatar style?** Gravatar (from user email hash) with Gravatar's default gray silhouette as fallback. Users set their email in settings.
