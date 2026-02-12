# Push Notifications for PWA

**Date:** 2026-02-11
**Status:** Brainstorm complete

## What We're Building

Push notifications for new chat messages using the standard W3C Web Push API. When a user has no active WebSocket connection (app is closed or backgrounded), the backend sends a push notification via the user's registered push subscription. Tapping the notification opens/focuses the app and navigates to the specific conversation.

### Scope

- **In scope:** New message notifications (private chat and topic messages), service worker for push event handling, backend Web Push protocol implementation (no third-party libraries), push subscription storage in SQLite, manual "Enable notifications" button.
- **Out of scope:** Task reminders, system announcements, per-topic mute, quiet hours, granular notification preferences, offline caching/cache-first strategies.

## Why This Approach

**W3C Web Push API with VAPID (self-implemented):**

- Industry standard, works across Chrome, Firefox, Edge, Safari 16.4+.
- No vendor dependency (no Firebase, no webpush-go) — the entire stack is self-contained.
- The Web Push protocol is straightforward: VAPID JWT signing (ECDSA P-256), payload encryption (ECDH + HKDF + AES-128-GCM), and an HTTP POST to the push service endpoint.
- All required crypto primitives are available in Go's standard library and the existing `golang.org/x/crypto` dependency.

**Rejected alternative:** Firebase Cloud Messaging — adds a Google dependency, requires Firebase project setup, and is overkill for single-purpose new-message notifications.

## Key Decisions

### 1. Trigger condition: No active WebSocket connection

The `ConnectionRegistry` already tracks active WebSocket connections per user. Push notifications are sent only when `registry.Count(userID) == 0`. This avoids duplicate notifications when the user is already viewing the app.

### 2. Browser permission: Manual button

An "Enable notifications" button will be placed in the UI (location TBD during planning). The user explicitly opts in. No automatic prompts on login or first message. This respects browser UX guidelines and avoids prompt fatigue.

### 3. No third-party Go libraries for Web Push

Implement the Web Push protocol from scratch using:
- `crypto/ecdsa` + `crypto/elliptic` (P-256) for VAPID key generation and JWT signing
- `crypto/ecdh` for key agreement with the push subscription's p256dh key
- `golang.org/x/crypto/hkdf` for key derivation
- `crypto/aes` + `crypto/cipher` (AES-128-GCM) for payload encryption
- `net/http` for sending the push message to the subscription endpoint

### 4. Notification click: Stay at `/` with HTMX internal navigation

iOS standalone PWAs treat any URL other than `start_url` (`/`) as an external link and open Safari. To keep the standalone experience:

1. Service worker `notificationclick` event focuses or opens a window at `/`.
2. If an existing client window is found, service worker sends a `postMessage` with the target chat URL (e.g., `/chat` or `/topics/{id}`). If no client window exists, open `/` with a query/hash hint (e.g., `/#/chat`) that client JS reads on load.
3. Client-side JS listens for `postMessage` (and checks the URL hash on page load) then uses `htmx.ajax()` to load the chat content, replacing the body.

This keeps the URL bar at `/` while navigating to the correct conversation. The hash-based fallback handles the case where the app was fully closed and a new window is opened.

### 5. VAPID key management

VAPID keys (ECDSA P-256 public/private pair) are configured via the existing config system. Keys are generated once and stored. The public key is exposed to the frontend for push subscription creation.

### 6. Subscription storage

A new `push_subscriptions` table in the core SQLite database stores per-user push subscription data (endpoint, p256dh key, auth secret). Multiple subscriptions per user are supported (different devices/browsers). Subscriptions are cleaned up when the push service returns a 404/410 (expired/unsubscribed).

### 7. Multi-device behavior

Push notifications are sent to *all* registered subscriptions when `registry.Count(userID) == 0` (no active WebSocket on any device). If one device has the app open (WebSocket connected), no device receives a push. This is the simplest correct behavior — no per-device connection tracking needed.

## Open Questions

- **Button placement:** Where should the "Enable notifications" button live? Header bar? Settings page? Chat page? (Decide during planning.)
- **Notification content:** How much of the message to include in the notification body? Full message or truncated preview? Should it include the sender name?
- **Service worker scope:** The service worker file must be served at root scope (`/sw.js`). Needs a dedicated route in the server since static files are under `/static/`.
- **Manifest bug:** The existing `layout.html` has `ref="manifest"` instead of `rel="manifest"`. Should be fixed as part of this work.
