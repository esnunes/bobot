---
title: "feat: Add Web Push Notifications for Chat Messages"
type: feat
date: 2026-02-11
brainstorm: docs/brainstorms/2026-02-11-push-notifications-brainstorm.md
---

# feat: Add Web Push Notifications for Chat Messages

## Overview

Add push notifications so users receive native OS notifications for new chat messages when the app is not open. Uses the W3C Web Push API with VAPID, implemented entirely with Go standard library crypto (no third-party push libraries). Includes a service worker for push event handling, backend encryption/delivery, and a manual opt-in button.

## Problem Statement

Users miss messages when the app is closed or backgrounded. There is no notification mechanism outside the active WebSocket connection. The PWA has manifest and icons but no service worker, so push is not possible today.

## Proposed Solution

Standard Web Push (RFC 8291 + RFC 8292) with self-implemented VAPID signing and payload encryption in Go. A service worker handles incoming push events and notification clicks. The backend checks `ConnectionRegistry.Count(userID)` and sends push only when zero WebSocket connections exist for the recipient.

## Technical Approach

### Architecture

```
┌─────────────┐    ┌──────────────┐    ┌──────────────────┐
│  Browser SW  │◄───│  Push Service │◄───│  Go Backend      │
│  (push event)│    │  (FCM/Mozilla)│    │  (encrypt+POST)  │
└──────┬───────┘    └──────────────┘    └────────┬─────────┘
       │                                         │
       │ showNotification()               ConnectionRegistry
       │                                  Count(userID)==0?
       ▼                                         │
┌──────────────┐                          ┌──────┴─────────┐
│  User clicks  │─── postMessage ────────►│  Client JS     │
│  notification │    or hash hint         │  htmx.ajax()   │
└──────────────┘                          └────────────────┘
```

**New files:**
- `push/push.go` — VAPID JWT, Web Push encryption, HTTP delivery
- `push/push_test.go` — Tests for crypto and delivery logic
- `server/push.go` — HTTP handlers for subscribe/unsubscribe API
- `server/push_test.go` — Handler tests
- `web/static/sw.js` — Service worker (push + notificationclick only)
- `web/static/push.js` — Client-side push manager (registration, subscribe, UI)

**Modified files:**
- `config/config.go` — Add VAPID config fields
- `db/core.go` — Add `push_subscriptions` table + CRUD methods
- `server/server.go` — Register new routes + serve `/sw.js` at root
- `server/chat.go` — Trigger push after broadcasts
- `web/templates/layout.html` — Fix `ref`→`rel` manifest bug, load `push.js`
- `web/templates/chat.html` — Add "Enable notifications" button to menu
- `web/templates/topic_chat.html` — Add "Enable notifications" button to menu

### Database Schema

Add to `db/core.go` `migrate()`:

```sql
CREATE TABLE IF NOT EXISTS push_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL UNIQUE,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user_id
    ON push_subscriptions(user_id);
```

The `UNIQUE` on `endpoint` handles the case where a user re-subscribes from the same browser — the old row is replaced via `INSERT OR REPLACE`.

### API Endpoints

**`POST /api/push/subscribe`** (authenticated via `sessionMiddleware`)
- Request body: `{"endpoint":"...","keys":{"p256dh":"...","auth":"..."}}`
- Stores/replaces the subscription for the authenticated user
- Response: `201 Created`

**`DELETE /api/push/subscribe`** (authenticated)
- Request body: `{"endpoint":"..."}`
- Deletes the subscription matching the endpoint for the authenticated user
- Response: `204 No Content`

**VAPID public key delivery:** Injected into `layout.html` as a `<meta>` tag (`<meta name="vapid-key" content="BASE64URL_KEY">`) when VAPID is configured, omitted when not. This avoids an extra API round-trip. The key is available on the `Server` struct (from config) and must be passed into template data for every authenticated page render — add a common helper or include it in the base template data that all page handlers use.

### Push Payload Structure

```json
{
  "title": "Alice",
  "body": "Hey, are you coming to the meeting?",
  "url": "/chat",
  "tag": "msg-12345"
}
```

- `title`: Sender display name (or "Bobot" for assistant). For topics: "Alice in #general".
- `body`: First 200 characters of the message content (plaintext, stripped of markdown).
- `url`: `/chat` for private messages, `/topics/{id}` for topic messages.
- `tag`: `msg-{messageID}` — browsers deduplicate notifications with the same tag.

Total payload stays well under 4KB after encryption.

### Notification Click Flow (iOS Standalone Compatible)

1. Service worker `notificationclick` → `clients.matchAll({type: "window"})`.
2. **If existing window found:** `client.focus()` + `client.postMessage({type: "navigate", url: "/chat"})`.
3. **If no window:** `clients.openWindow("/#navigate=/chat")` — opens at `/` with hash hint.
4. Client JS in `push.js` listens for:
   - `navigator.serviceWorker.onmessage` → reads `url`, calls `htmx.ajax("GET", url, {target: "body"})`.
   - On page load, checks `location.hash` for `#navigate=` prefix → validates the path (only `/chat` or `/topics/{id}`), calls `htmx.ajax()`, clears hash.

This keeps the URL at `/` so iOS standalone mode is preserved.

### Async Push Sending

Push HTTP requests must not block the WebSocket message loop. Create a `PushSender` in `push/push.go`:

```go
type PushSender struct {
    db         *db.CoreDB
    vapidKey   *ecdsa.PrivateKey
    vapidPub   []byte // 65-byte uncompressed public key
    subject    string // "mailto:..." for VAPID sub claim
    httpClient *http.Client
}
```

The chat handlers call `go s.pushSender.NotifyUser(ctx, userID, payload)` — a fire-and-forget goroutine. Inside `NotifyUser`:
1. Look up all subscriptions for `userID` from DB.
2. For each subscription, encrypt and POST.
3. On 404/410 response, delete the stale subscription.
4. Log errors for 401/429/5xx but do not retry (keep it simple for v1).

### VAPID and Encryption (RFC 8292 + RFC 8291)

Implemented in `push/push.go` using only Go standard library + `golang.org/x/crypto/hkdf`:

**VAPID JWT signing (ES256):**
1. Build JWT header `{"typ":"JWT","alg":"ES256"}` + claims `{"aud": <endpoint origin>, "exp": <now+12h>, "sub": <config subject>}`.
2. Sign with `ecdsa.Sign()` using the VAPID private key.
3. Format: `Authorization: vapid t=<jwt>,k=<base64url pubkey>`.

**Payload encryption (aes128gcm):**
1. Generate ephemeral P-256 key pair + 16-byte random salt.
2. ECDH shared secret: `ephemeral_private × subscription.p256dh`.
3. IKM: `HKDF(auth_secret, shared_secret, "WebPush: info\0" || ua_pub || as_pub)` → 32 bytes.
4. CEK: `HKDF(salt, IKM, "Content-Encoding: aes128gcm\0")` → 16 bytes.
5. Nonce: `HKDF(salt, IKM, "Content-Encoding: nonce\0")` → 12 bytes.
6. Encrypt: `AES-128-GCM(CEK, nonce, plaintext || 0x02)`.
7. Body: `salt(16) || rs(4, BE) || idlen(1) || ephemeral_pub(65) || ciphertext`.

**HTTP POST:**
- `POST <endpoint>` with headers: `Authorization: vapid ...`, `Content-Encoding: aes128gcm`, `Content-Type: application/octet-stream`, `TTL: 86400`.

### Key Design Decisions

**Multi-device:** Push is sent to ALL subscriptions when `Count(userID) == 0`. If one device has an active WebSocket, no device gets push. This is the simplest approach — no device ID tracking needed. Users who need per-device granularity can accept this trade-off for v1.

**Logout cleanup:** On logout, the client calls `PushManager.unsubscribe()` and `DELETE /api/push/subscribe` before clearing the session. This prevents stale subscriptions from leaking notifications to the wrong user on a shared browser.

**Blocked users:** Skip push for blocked users — check `user.Blocked` before sending.

**Bobot responses:** Send push for assistant responses in both private and topic chat. The user asked a question and deserves notification of the answer.

**No fetch caching in SW:** The service worker handles only `push` and `notificationclick` events. No `fetch` event interception, no caching strategy. This avoids interference with HTMX and WebSocket behavior.

**VAPID not configured = push disabled:** If `BOBOT_VAPID_PUBLIC_KEY` and `BOBOT_VAPID_PRIVATE_KEY` are not set, push is silently disabled. No `<meta name="vapid-key">` tag is rendered, `push.js` detects this and skips service worker registration. No errors.

**Endpoint validation:** Before storing a subscription, validate that the endpoint URL uses HTTPS. This mitigates basic SSRF risk without maintaining a push service domain allowlist.

## Implementation Phases

### Phase 1: Backend — Crypto, Config, Database

**Goal:** The `push` package can encrypt a payload and deliver it to a push service endpoint. DB stores subscriptions.

- [x] Add VAPID config to `config/config.go`
  - `BOBOT_VAPID_PUBLIC_KEY` (base64url-encoded 65-byte uncompressed P-256 public key)
  - `BOBOT_VAPID_PRIVATE_KEY` (base64url-encoded 32-byte raw private key scalar)
  - `BOBOT_VAPID_SUBJECT` (mailto: or https: URL)
  - All optional; push disabled if absent
- [x] Add `push_subscriptions` table to `db/core.go` `migrate()`
- [x] Add DB methods to `db/core.go`:
  - `SavePushSubscription(userID int64, endpoint, p256dh, auth string) error`
  - `DeletePushSubscription(endpoint string) error`
  - `DeletePushSubscriptionsByUser(userID int64) error`
  - `GetPushSubscriptions(userID int64) ([]PushSubscription, error)`
- [x] Create `push/push.go`:
  - `NewPushSender(cfg, db) *PushSender`
  - VAPID JWT generation (ES256 signing)
  - Web Push encryption (aes128gcm per RFC 8291)
  - `Send(subscription, payload []byte) error` — encrypt + POST
  - `NotifyUser(ctx, userID int64, payload []byte)` — look up subscriptions, send to each, clean up stale
- [x] Create `push/push_test.go`:
  - Test VAPID JWT structure and signature verification
  - Test payload encryption with known test vectors (RFC 8291 Section 5)
  - Test HTTP delivery with httptest server (201, 410 → delete, 429 → log)

### Phase 2: Backend — API Handlers and Chat Integration

**Goal:** Subscription CRUD endpoints work. Chat messages trigger push for offline users.

- [x] Create `server/push.go`:
  - `handlePushSubscribe` — decode JSON body, validate HTTPS endpoint, call `db.SavePushSubscription`
  - `handlePushUnsubscribe` — decode JSON body, call `db.DeletePushSubscription`
- [x] Register routes in `server/server.go`:
  - `POST /api/push/subscribe` → `s.sessionMiddleware(s.handlePushSubscribe)`
  - `DELETE /api/push/subscribe` → `s.sessionMiddleware(s.handlePushUnsubscribe)`
  - `GET /sw.js` → serve `web/static/sw.js` with `Content-Type: application/javascript` and `Service-Worker-Allowed: /`
- [x] Add `pushSender *push.PushSender` to `Server` struct (nil when VAPID not configured)
- [x] Integrate push triggers in `server/chat.go`:
  - After `s.connections.Broadcast(userID, ...)` in private chat: if `s.pushSender != nil && s.connections.Count(userID) == 0`, call `go s.pushSender.NotifyUser(ctx, userID, payload)`
  - After `s.broadcastToTopic(topicID, ...)` in topic chat: for each member except sender, if `s.connections.Count(memberID) == 0`, call `go s.pushSender.NotifyUser(ctx, memberID, payload)`
- [x] Add cleanup to logout flow: `db.DeletePushSubscriptionsByUser(userID)` in `handleLogout`
- [x] Create `server/push_test.go`:
  - Test subscribe handler (valid input, missing fields, non-HTTPS endpoint)
  - Test unsubscribe handler

### Phase 3: Frontend — Service Worker, Push Manager, UI

**Goal:** Users can enable push notifications and receive/click them.

- [x] Fix manifest bug in `web/templates/layout.html`: `ref="manifest"` → `rel="manifest"`
- [x] Add `<meta name="vapid-key"` content="{{.VAPIDPublicKey}}">` to `layout.html` (conditionally rendered when VAPID is configured)
- [x] Add `<script src="/static/push.js" defer></script>` to `layout.html`
- [x] Create `web/static/sw.js`:
  - `push` event handler: parse JSON payload, call `self.registration.showNotification(title, {body, icon, tag, data: {url}})`
  - `notificationclick` event handler: close notification, `clients.matchAll()`, focus existing window + `postMessage({type: "navigate", url})` or `clients.openWindow("/#navigate=" + url)`
  - No `fetch` event handler
- [x] Create `web/static/push.js`:
  - On load: read `<meta name="vapid-key">`. If absent, do nothing (push disabled).
  - Register service worker: `navigator.serviceWorker.register("/sw.js")`
  - `enablePush()`: request permission → `PushManager.subscribe()` → POST to `/api/push/subscribe`
  - `disablePush()`: `PushManager.unsubscribe()` → DELETE to `/api/push/subscribe`
  - Listen for `navigator.serviceWorker.onmessage` → on `{type: "navigate"}`, call `htmx.ajax("GET", url, {target: "body"})`
  - On page load, check `location.hash` for `#navigate=` → validate path is `/chat` or `/topics/{id}` (prevent open redirect), call `htmx.ajax()`, clear hash via `history.replaceState`
  - Listen for `bobot:logout` event → call `disablePush()` before logout completes
  - Export state check: `isPushEnabled()` reads `PushManager.getSubscription()` to toggle button label
- [x] Add "Enable notifications" button to menu overlay in `chat.html` and `topic_chat.html`:
  - Shows "Enable notifications" when not subscribed, "Disable notifications" when subscribed
  - Hidden when Push API not supported or VAPID not configured
  - Handles "denied" permission state: show message guiding user to browser settings

## Acceptance Criteria

### Functional Requirements

- [ ] User can click "Enable notifications" in the menu, grant permission, and receive push notifications for new messages
- [ ] Push notifications are sent only when the user has zero active WebSocket connections
- [ ] Clicking a notification opens/focuses the app and navigates to the correct conversation (private chat or topic)
- [ ] iOS standalone mode is preserved (URL stays at `/`)
- [ ] User can disable notifications from the menu
- [ ] Logging out cleans up push subscriptions
- [ ] Stale subscriptions (404/410 from push service) are automatically removed
- [ ] Push is silently disabled when VAPID keys are not configured
- [ ] Blocked users do not receive push notifications

### Non-Functional Requirements

- [ ] Push sending does not block the WebSocket message loop (async goroutine)
- [ ] Payload encryption follows RFC 8291 (aes128gcm)
- [ ] VAPID follows RFC 8292 (ES256 JWT)
- [ ] Subscription endpoint must be HTTPS (SSRF mitigation)
- [ ] No third-party Go libraries for Web Push (only stdlib + golang.org/x/crypto)

### Quality Gates

- [ ] `push/push_test.go` covers VAPID signing, payload encryption, and HTTP delivery
- [ ] `server/push_test.go` covers subscribe/unsubscribe handlers
- [ ] Existing tests still pass (no regressions)

## Dependencies & Prerequisites

- Go `crypto/ecdsa`, `crypto/ecdh`, `crypto/aes`, `crypto/cipher`, `crypto/sha256` — all in stdlib
- `golang.org/x/crypto/hkdf` — already in `go.mod`
- Browser Push API support (Chrome 50+, Firefox 44+, Safari 16.4+, Edge 17+)
- VAPID key pair must be generated and configured as env vars before push is functional

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| Crypto implementation bug | Push silently fails | Test with RFC 8291 test vectors; test against real push service |
| iOS Safari restrictions | Push only works when added to home screen | Document requirement; graceful degradation |
| WebSocket reconnection race | Duplicate notification | Use notification `tag` for dedup |
| Push service rate limiting | Notifications delayed | Log 429 responses; no retry in v1 |

## Future Considerations (Out of Scope)

- Per-topic mute / notification preferences
- Unread message count badge (`navigator.setAppBadge()`)
- Retry queue for failed push deliveries
- Per-device push decisions (send push only to disconnected devices)
- Offline caching in service worker

## References

### Specs
- [RFC 8292: VAPID](https://www.rfc-editor.org/rfc/rfc8292)
- [RFC 8291: Web Push Encryption](https://www.rfc-editor.org/rfc/rfc8291)
- [RFC 8188: aes128gcm Content Coding](https://httpwg.org/specs/rfc8188.html)
- [RFC 8030: HTTP Push](https://www.rfc-editor.org/rfc/rfc8030)

### Internal References
- Brainstorm: `docs/brainstorms/2026-02-11-push-notifications-brainstorm.md`
- Connection registry: `server/connections.go:18-71`
- Chat broadcast: `server/chat.go:86-252`
- Config pattern: `config/config.go:59-99`
- DB migration: `db/core.go:108-381`
- WS manager: `web/static/ws-manager.js`
- Layout template: `web/templates/layout.html`
