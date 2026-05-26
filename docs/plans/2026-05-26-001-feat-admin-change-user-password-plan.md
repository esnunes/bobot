---
title: "feat: Admin change a user's password"
type: feat
status: active
date: 2026-05-26
deepened: 2026-05-26
origin: docs/brainstorms/2026-05-26-admin-change-user-password-requirements.md
---

# feat: Admin change a user's password

## Overview

Add an admin-only "Change password" action to the user detail page (`GET /admin/users/{id}`). An admin types a new password (with confirmation), the server validates and persists a bcrypt hash, and the target user's existing sessions are revoked so they must re-login. This is the **first admin-side mutation** in the app — there is no prior admin write handler to copy, so the handler is modeled on the existing admin read handler (`handleAdminUserPage`) for id-parsing/guards plus the user-settings writers (`handleUpdateDisplayName`) for form parsing and response shape.

## Problem Frame

There is currently no in-app way to change a user's password; it can only be set at signup or via the admin-creation CLI, and `db/core.go` has no password-update method. An admin's only recourse for a forgotten password is direct DB manipulation. This closes that operational gap. (See origin: `docs/brainstorms/2026-05-26-admin-change-user-password-requirements.md`.)

**Scope note on compromised accounts:** because revocation is eventual (see R8 / Key Technical Decisions), this feature targets forgotten-password and routine resets. It does *not* promptly lock out an active attacker — their already-issued token survives until the refresh window and a live WebSocket persists until disconnect. Real-time lockout of a compromised session is an explicit non-goal here (see Scope Boundaries); a future block + immediate-cutoff mechanism would be the right tool for that.

## Requirements Trace

- R1. "Change password" section/form on `web/templates/admin_user.html` (page already gated by `adminMiddleware`).
- R2. New-password + confirm-password fields; admin types the password directly.
- R3. Clear inline confirmation on success; specific reason on failure (mismatch, too short, server error) with no change applied.
- R4. New admin-only endpoint, enforced by `adminMiddleware` (non-admins get 403).
- R5. The two fields must match; mismatch is rejected with a clear error and no change.
- R6. Minimum length reused from signup (8 chars); shorter is rejected with a clear error.
- R7. Hash via `auth.HashPassword`; persist via a new `UpdateUserPassword(userID, passwordHash)` DB method modeled on `UpdateUserEmail`.
- R8. On success, revoke the target's sessions via `CreateSessionRevocation(userID, reason)`. Revocation is **eventual** (takes effect at the user's next token refresh, up to ~one session duration); live WebSocket chats persist until disconnect.
- R9. Endpoint validates the target user: 404 for an unknown user ID or the reserved bobot system user (mirroring `handleAdminUserPage`). Existence is enforced at the handler *before* the update, not via a DB-layer zero-row check — a conscious amendment to the origin's R9 (see Key Technical Decisions).
- R10. A confirmation step naming the target user and warning about sign-out gates the submit; the password changes only after the admin confirms.

## Scope Boundaries

- No self-service password change for regular users (admin-initiated only).
- No forgot-password / email reset flow (no email infra).
- No force-change-on-next-login.
- No immediate session cut-off and no closing of live WebSocket connections — revocation is eventual by design.
- Promptly securing a *compromised* account (cutting an active hostile session in real time) is a non-goal: eventual revocation means an attacker's existing token and live WebSocket survive until the refresh window / disconnect. No real-time-cutoff mechanism exists today.
- No generated/temporary passwords; no complexity rules beyond minimum length.
- No audit-log feature; a non-sensitive `slog` line is acceptable (must not log the password/hash/body).

## Context & Research

### Relevant Code and Patterns

- **Route registration** — `server/server.go:152-155` (admin block) and `:174-176` (settings writers). Stdlib `net/http.ServeMux` with Go 1.22+ method+pattern routing; middleware is explicit function composition: `s.sessionMiddleware(s.adminMiddleware(handler))`.
- **`adminMiddleware`** — `server/server.go:269-278`: 403 when `userData.Role != "admin"`.
- **`sessionMiddleware`** — `server/server.go:207-267`: loads user, checks `Blocked` + `HasSessionRevocation`. Revocation is only checked on the token reissue path (`:230-244`), not every request — this is the source of R8's eventual-revocation behavior (acknowledged in `server/server_test.go:460-464`).
- **Admin read handler / guards to mirror** — `handleAdminUserPage` `server/admin.go:53-70`: `r.PathValue("id")` → `strconv.ParseInt` (400 on bad id) → `userID == db.BobotUserID` 404 → `GetUserByID` (any error → 404).
- **Form-write handler to mirror** — `handleUpdateDisplayName` `server/pages.go:697-717`: `r.ParseForm()` → `r.FormValue(...)` → validate → DB call → `http.StatusNoContent`. Validation failures use `http.Error(w, msg, http.StatusBadRequest)`.
- **DB update methods to mirror** — `UpdateUserEmail`/`UpdateUserDisplayName` `db/core.go:1516-1532`: single `c.db.Exec("UPDATE users SET ... WHERE id = ?", ...)`, return only `error`, no `RowsAffected` check. The `users` table already has `password_hash`.
- **Session revocation** — `CreateSessionRevocation(userID, reason)` `db/core.go:1401-1407` (existing caller: `handleLogout` with `"logout_all"`, `server/pages.go:370`).
- **Password helpers** — `auth/password.go`: `HashPassword`/`CheckPassword` (bcrypt, default cost).
- **Length validation** — `validatePassword` `server/signup.go:173-179` returns an i18n key (`"signup.error.password_min"`) or empty; `8` is a hardcoded literal, no shared constant. Same package as the new handler, so callable directly.
- **Template** — `web/templates/admin_user.html`: server-rendered Go `html/template`, sections as `<details class="context-section">`, HTMX for navigation, currently zero forms/mutations. i18n via `t` func (`{{t .Lang "key"}}`, `$.Lang` inside `range`); catalog under `i18n/`.
- **Confirm-password idiom** — `web/templates/signup.html:5-25`: `htmx:confirm` listener + `data-*` localized mismatch message; `minlength="8"`; server-side `validatePassword` is source of truth.
- **Inline feedback idiom** — `web/static/settings.js:18-96`: vanilla `fetch`, `submitBtn.disabled` during request, transient saved-message on `resp.ok`; reads localized strings from a `<script data-i18n>` JSON blob (`:8-9`). Only `console.error`s on failure today — this feature needs a *visible* error state.
- **Cookie/CSRF** — `setSessionCookie` `server/server.go:280-291`: `HttpOnly`, `SameSite=Lax`, `Secure` only when `BaseURL` is https. No CSRF token mechanism anywhere in the app.
- **Tests** — stdlib `testing` + `net/http/httptest`, no testify. Admin/role requests: mint a token with `s.session.CreateToken(userID, "admin")` (signature `CreateToken(userID int64, role string, lang ...string)` — `lang` is variadic, so the 2-arg form is valid) and attach as a `session` cookie (`server/server_test.go:202,503`; `server/push_test.go:12-29`). DB mutation test template: `TestCoreDB_BlockUser` `db/core_test.go:313-329`.

### Institutional Learnings

- `docs/solutions/architecture-patterns/admin-context-inspection-dashboard.md` — admin features register as `sessionMiddleware(adminMiddleware(handler))` and live in `server/admin.go`; test that non-admins get 403.
- `docs/akb/server/signup.go.md` — signup is the reference password-write flow (parse → `validatePassword` → `HashPassword` → DB write → inline error / redirect on success). Reuse `validatePassword`; don't reinvent bcrypt.
- `docs/akb/server/server.go.md`, `docs/akb/auth/session.go.md` — forced re-login is done via `session_revocations`, **not** cookie clearing (tokens are stateless, client-held). Revocation enforced only on the reissue path → logout latency ≈ token duration/refresh window, not instant. `revoked_at` defaults to `CURRENT_TIMESTAMP` (SQLite, second-resolution) and `HasSessionRevocation` matches `revoked_at > tokenIssuedAt` (strict `>`), so a token issued in the same whole second as the reset may *not* be caught — acceptable given the already-eventual semantics, but don't claim "every earlier token is guaranteed caught."
- `docs/akb/auth/context.go.md` — authorize via the acting admin from `auth.UserDataFromContext`; the target user comes from the route. Never trust a form-supplied user id for authorization.
- No prior `docs/solutions/` write-up exists for password change / session revocation; this is a good candidate to document afterward.

### External References

- None. Local patterns (bcrypt hashing, session revocation, admin middleware, form handlers, test harness) are well-established and directly applicable; external research was skipped.

## Key Technical Decisions

- **Endpoint shape**: `POST /admin/users/{id}/password`, wrapped `sessionMiddleware(adminMiddleware(...))`, handler `handleAdminUpdateUserPassword` in `server/admin.go`. Rationale: consistent with the existing `/admin/users/{id}` route and admin handler location; `adminMiddleware` already gives R4's 403.
- **Reuse `validatePassword` directly** (not a copied `8` literal). Rationale: same package, single source of truth — satisfies R6 and resolves the brainstorm's "duplicated literal" concern without inventing a new constant/convention.
- **Existence check satisfies R9's 404 clause; the origin's "zero-row = error" clause is consciously amended.** The handler loads the target via `GetUserByID` (404 on miss) and guards `BobotUserID` *before* the update, so on the happy path the `Exec` can never hit zero rows. `UpdateUserPassword` therefore mirrors its siblings (no `RowsAffected` check), keeping `db/core.go` consistent. **Amendment:** the origin's R9 asked the DB layer itself to treat a zero-row UPDATE as an error; this plan enforces existence at the handler instead. The only residual gap is a TOCTOU window — the target row deleted between `GetUserByID` and `UpdateUserPassword` would report success with no change. Accepted: single-binary local SQLite, low-frequency admin action, no concurrent/remote user-deletion path exists today. Revisit (add a `RowsAffected` check) if user deletion ever becomes concurrent.
- **Revoke with reason `"admin_password_reset"`** after a successful update. Rationale: matches the existing `session_revocations` mechanism (R8); distinct reason aids future debugging.
- **Eventual revocation accepted** (per origin): no per-request revocation check, no WebSocket teardown. Rationale: low-frequency admin action; immediate cut-off isn't worth the per-request DB cost and connection-teardown logic.
- **Frontend = vanilla `fetch` + inline feedback** (settings.js style), not full-page HTMX re-render (signup style). Rationale: `admin_user.html` is data-heavy; re-rendering re-runs all its DB lookups on every submit/error. A scoped `fetch` returning 204/text avoids that. Confirm-password mismatch is checked client-side (signup idiom) and again server-side (source of truth).
- **Confirmation step via `window.confirm()`** naming the target user and warning about sign-out (R10), in the form's submit handler before the `fetch`. Rationale: lightweight, no new dependency; the target display name is already in template scope.
- **Admin self-reset is in scope; the acting admin is also eventually signed out.** Revocation applies to the acting admin's own sessions too — they keep the *current* session through the refresh window (so R3's inline success message renders normally, no redirect needed) but are logged out at their next token refresh, exactly like any other target. No special-case handling is required. Decided (implemented in Unit 3): when the target is the acting admin, `window.confirm()` additionally warns that they will be signed out at their next refresh.
- **Resetting an admin target is in scope and allowed.** One admin may reset another admin's password; the app's trust model treats every admin as fully trusted and there is no audit trail (out of scope per origin). No role check beyond `adminMiddleware` is added. This carries an accepted lateral-takeover / no-accountability risk (see Risks) — surfaced explicitly so it's a decision, not an oversight.
- **CSRF**: inherit the app-wide `SameSite=Lax`-only posture; no token. Rationale: consistent with every other mutation in the app; documented accepted risk.
- **No step-up re-auth**: an active admin session is sufficient authorization. Rationale: matches the app's existing trust model; documented accepted risk.
- **HTTPS is required in production for this endpoint.** It transmits a cleartext password in the POST body, and the session cookie only gets `Secure` when `BaseURL` is https (`server/server.go:281`). Over plain http (or behind an http-terminating proxy) both the password and the cookie are exposed. Decision: do *not* add code-level refusal for an http `BaseURL` (out of step with the rest of the app), but call the HTTPS requirement out explicitly as a hard deployment constraint rather than leaving the protection implicit (see Operational Notes).

## Open Questions

### Resolved During Planning

- *Route/verb and form pattern?* → `POST /admin/users/{id}/password`; vanilla `fetch` returning 204 (success) / `http.Error` text (failure), with inline feedback. (Deferred item from origin.)
- *Error/success display on a page with no form pattern?* → Add a `<details>` section with a form; success/error shown inline via JS (extend settings.js's success-only pattern with a visible error element).
- *Mismatch + length validation location?* → Server-side is source of truth (`validatePassword` + explicit match check); client-side mismatch check mirrors signup for UX only.
- *Minimum-length source of truth / shared constant?* → Reuse `validatePassword` directly; no new constant.
- *CSRF posture?* → Inherit `SameSite=Lax`, no token (accepted risk).
- *Step-up re-auth?* → Not required (accepted risk).
- *Target-user edge cases?* → 404 on unknown id and `BobotUserID`; existence check before update prevents silent no-op.
- *Blocked target user?* → Password is set, but login stays blocked (the `Blocked` check in `sessionMiddleware`/login is unchanged). Success-criterion wording reflects this.
- *bcrypt 72-byte limit?* → Resolved via the server-side 72-byte (byte-length) max guard (Unit 2): overlong inputs are rejected with a 400 before hashing. Note bcrypt *errors* (`ErrPasswordTooLong`) on > 72 bytes rather than silently truncating, so the guard pre-empts that error path.
- *Admin self-reset / admin-on-admin reset?* → Both in scope (see Key Technical Decisions): the acting admin is also eventually signed out; resetting another admin is allowed under the app's all-admins-trusted model, with the lateral-takeover/no-audit risk accepted.

### Deferred to Implementation

- Exact i18n key names and English copy for the new strings (label, button, mismatch, success, length, generic error) — chosen when editing the `i18n/` catalog.
- Whether the handler translates validation messages via `i18n.T` (admin's `Lang` from context) or returns plain English like `handleUpdateDisplayName` — decide while wiring the handler; lean toward i18n for parity with signup.
- Exact placement/order of the new `<details>` section within `admin_user.html`.

## Implementation Units

- [ ] **Unit 1: `UpdateUserPassword` DB method**

**Goal:** Persist a new bcrypt hash for a user.

**Requirements:** R7

**Dependencies:** None

**Files:**
- Modify: `db/core.go` (add method near the other `Update*` methods, ~`:1532`)
- Test: `db/core_test.go`

**Approach:**
- Mirror `UpdateUserEmail` exactly: `UPDATE users SET password_hash = ? WHERE id = ?`, return only `error`, no `RowsAffected` check (handler guarantees the user exists — see Key Decisions).

**Patterns to follow:**
- `UpdateUserEmail` / `UpdateUserDisplayName` (`db/core.go:1516-1532`); test template `TestCoreDB_BlockUser` (`db/core_test.go:313-329`).

**Test scenarios:**
- Happy path: create a user, call `UpdateUserPassword(id, "newhash")`, then `GetUserByID(id)` returns `PasswordHash == "newhash"`.
- Edge case: calling with a non-existent id returns no error (documents the convention that the handler, not the DB method, enforces existence).

**Verification:**
- `db` package tests pass; stored `password_hash` reflects the new value.

- [ ] **Unit 2: Admin password-change handler + route**

**Goal:** Server-side endpoint that validates input, updates the hash, and revokes the target's sessions.

**Requirements:** R4, R5, R6, R7, R8, R9

**Dependencies:** Unit 1

**Files:**
- Modify: `server/admin.go` (add `handleAdminUpdateUserPassword`)
- Modify: `server/server.go` (register `POST /admin/users/{id}/password` in the admin block, ~`:155`)
- Test: `server/admin_test.go` (new file)

**Approach:**
- Parse + guard the target exactly like `handleAdminUserPage:54-70`: `r.PathValue("id")` → `ParseInt` (400 on bad id) → `BobotUserID` 404 → `GetUserByID` (404 on error). This satisfies R9 and guarantees a real target before any write.
- Cap the body with `http.MaxBytesReader(w, r.Body, 4096)` *before* parsing — the form is small and expected as `application/x-www-form-urlencoded`; this fails closed against an oversized body rather than reading an unbounded value into memory and on into bcrypt.
- `r.ParseForm()`; read `password` and `confirm_password`.
- If `password != confirm_password` → 400 with a mismatch message (R5).
- `validatePassword(password)`; if it returns a non-empty i18n key → 400 with the (translated) message (R6).
- Enforce a server-side **maximum** length measured in bytes — `len([]byte(password)) > 72` → 400 if exceeded (72 is bcrypt's input limit). The vendored bcrypt (`golang.org/x/crypto` v0.48.0) **returns `ErrPasswordTooLong` for input > 72 bytes — it does not silently truncate** (`bcrypt/bcrypt.go:96-97`); rejecting > 72 bytes before hashing means `HashPassword` never hits that error on this path, and also bounds bcrypt input alongside the body cap. Byte vs. character matters here: the client `maxlength` (Unit 3) counts UTF-16 code units, so it is only an approximate hint — this server byte-check is authoritative for multibyte input.
- `auth.HashPassword(password)` → `s.db.UpdateUserPassword(targetID, hash)` (R7); 500 on error.
- `s.db.CreateSessionRevocation(targetID, "admin_password_reset")` (R8); 500 on error.
- Respond `http.StatusNoContent` on success (mirrors `handleUpdateDisplayName`).
- Authorization comes from `adminMiddleware`; never read authorization from the form. The acting admin (`auth.UserDataFromContext`) is used only for the response `Lang` and the (non-sensitive) `slog` line. The `slog` line logs acting-admin id, target id, and outcome — never the password, hash, or body.

**Patterns to follow:**
- Guards: `handleAdminUserPage` (`server/admin.go:53-70`). Form handling + response: `handleUpdateDisplayName` (`server/pages.go:697-717`). Revocation: `handleLogout` (`server/pages.go:370`).

**Test scenarios:**
- Happy path (admin): valid matching password → 204; `GetUserByID` hash satisfies `auth.CheckPassword(newPlain, hash)`; a `session_revocations` row exists for the target afterward.
- Happy path (admin target): an admin resetting *another admin's* password → 204 (confirms admin-on-admin reset is allowed — see Key Technical Decisions).
- Error path (authz): non-admin session → 403 (no change).
- Error path (validation): mismatched fields → 400, hash unchanged.
- Error path (validation): password shorter than 8 → 400, hash unchanged.
- Error path (validation): password longer than 72 bytes → 400, hash unchanged.
- Error path (no side effect): any failed validation (mismatch / too short / too long) creates **no** `session_revocations` row for the target.
- Edge case: unknown user id → 404, no revocation row.
- Edge case: `id == 0` (`BobotUserID`) → 404.
- Edge case: non-numeric id → 400.
- Integration: after a successful change, a token issued *before* the change is rejected on its reissue path (exercises the `CreateSessionRevocation` → `HasSessionRevocation` interaction), confirming R8's eventual revocation. **This must be made deterministic.** With the default test config (`Duration` 30m, `RefreshThreshold` 5m in `setupTestServer:25-28`), a freshly minted token is *not* within the refresh window, so `NeedsReissue` is false and the revocation check at `server/server.go:230-244` is never reached — the request passes through 200 and proves nothing (this is exactly why the existing `TestSessionMiddleware_RevokedSession` accepts both 200 and 401, `server_test.go:460-464`). Build a dedicated `Server` whose `SessionService` has `RefreshThreshold >= Duration` (via a custom `config.SessionConfig`, not `setupTestServer`'s defaults), so every token is always inside the refresh window → `NeedsReissue` always true; then assert the pre-revocation token is rejected on its next request.

**Verification:**
- `server` package tests pass; all status codes and the revocation side effect hold as enumerated.

- [ ] **Unit 3: Admin user-detail UI (form, confirm, inline feedback, i18n)**

**Goal:** Let an admin enter and submit a new password from the user detail page, with a confirmation step and visible success/error feedback.

**Requirements:** R1, R2, R3, R5 (client-side), R10

**Dependencies:** Unit 2

**Files:**
- Modify: `web/templates/admin_user.html` (add a "Change password" `<details class="context-section">` with new/confirm password inputs, a submit button, and success/error message elements; expose the target id and display name via attributes; include the new script via `<script src>`)
- Modify: `server/admin.go` (`handleAdminUserPage` must set `PageData.CurrentUserID` from `auth.UserDataFromContext` so the template can detect self-reset — currently unset; see Approach)
- Add: a new admin-page-scoped static JS file (e.g. `web/static/admin_user.js`), included by `admin_user.html`. **Do not extend `web/static/settings.js`** — it early-returns unless `[data-page="settings"]` matches and is only included by `settings.html` (verified `web/static/settings.js:11-12`), so it never runs on the admin page. Mirror its fetch/disabled-button/transient-message idiom in the new file (submit handler: `window.confirm()` naming the target user → mismatch check → disabled button → `fetch` POST → inline success on `resp.ok`, inline error text from `resp.text()` otherwise).
- Modify: `i18n/` catalog (new keys: section/label, button, confirm warning, mismatch, length error, success, generic error)

**Approach:**
- Mirror the signup confirm-password inputs (`type="password"`, `minlength="8"`, `maxlength="72"`, localized placeholders) and the settings.js fetch/disabled-button/transient-message idiom (mirrored into the new admin script — see Files), adding a *visible* error element (settings.js currently only `console.error`s). `maxlength="72"` is only an approximate client hint — HTML `maxlength` counts UTF-16 code units, not bytes, so the server byte-check (Unit 2) is authoritative for multibyte input.
- The submit handler: `e.preventDefault()`; if `password !== confirm_password` show the localized mismatch inline and stop; else `window.confirm(<localized warning naming the user>)`; if confirmed, disable the button and `fetch('/admin/users/{id}/password', { method:'POST', headers:{'Content-Type':'application/x-www-form-urlencoded'}, body:'password=...&confirm_password=...' })`; on `resp.ok` show success and clear the fields, otherwise show `await resp.text()` as the inline error; re-enable the button in `finally`.
- Self-reset warning: expose whether the target is the acting admin by comparing the route's target id with the acting admin's id. **The acting admin's id is NOT currently in this template's data** — `handleAdminUserPage` populates only `AdminUserDetail`, leaving `PageData.CurrentUserID` at its zero value, which collides with `BobotUserID=0` (verified: `server/admin.go:182`, `server/pages.go:175`). So `handleAdminUserPage` must be updated to set `CurrentUserID` from `auth.UserDataFromContext` (see Files). When target == acting admin, the confirm warning additionally notes they will be signed out at their next refresh; otherwise it names the target user as usual.
- Add `autocomplete="new-password"` on both inputs so the browser doesn't autofill the admin's own credentials. Use real `<label>`s, not placeholder-only (origin a11y note).

**Execution note:** Frontend behavior here is browser-only; covered by manual verification rather than Go tests (no JS test harness in the repo).

**Patterns to follow:**
- `web/templates/signup.html:5-25` (confirm idiom, `data-*` localized message); `web/static/settings.js:18-96` (fetch + disabled button + transient message + `<script data-i18n>` strings); `t`/`$.Lang` i18n usage in `admin_user.html`.

**Test scenarios:**
- Test expectation: none (no Go test) — browser-only behavior. Manual verification below.

**Verification:**
- On the user detail page, entering matching valid passwords and confirming shows the success message and clears the fields; the target user can then log in with the new password.
- Mismatched fields show the inline mismatch message and do not submit.
- A too-short password is rejected with the server's message shown inline.
- Cancelling the `confirm()` dialog makes no request.
- Submitting disables the button until the request resolves (no double-submit).

## System-Wide Impact

- **Interaction graph:** Reuses `sessionMiddleware` + `adminMiddleware` unchanged. New write path: handler → `UpdateUserPassword` → `CreateSessionRevocation`. First admin-side mutation — establishes the pattern future admin actions (e.g. the unwired `BlockUser`/`UnblockUser`) can follow.
- **Error propagation:** Validation/authz/target errors surface as HTTP status + text the client renders inline; DB errors return 500 with a generic message (no internals leaked).
- **State lifecycle risks:** `UpdateUserPassword` and `CreateSessionRevocation` are two separate `Exec`s (no transaction), matching existing convention. If the revocation insert fails after the hash update, the handler returns 500 while the password is already changed; acceptable (the new password works; only the forced-logout side effect is missing) — documented under Risks.
- **API surface parity:** Admin-only; no user-facing self-service endpoint is added (out of scope). The unwired `BlockUser`/`UnblockUser` are not addressed here.
- **Integration coverage:** The token-issued-before-change → rejected-on-reissue scenario (Unit 2) is the cross-layer behavior unit-level mocks wouldn't prove.
- **Unchanged invariants:** Login, signup, the `Blocked` check, session encryption, and all existing admin read routes are untouched. Revocation timing semantics are unchanged (still reissue-path only).

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Eventual (not immediate) revocation leaves a compromised session valid for up to ~one session duration | Accepted per origin; documented in R8 and success criteria. Immediate cut-off is explicitly out of scope. |
| Live WebSocket chat survives revocation until disconnect | Accepted/out of scope; noted so it isn't mistaken for a bug. |
| No CSRF token (SameSite=Lax only) on a high-value mutation | Accepted; consistent with the whole app. Endpoint requires an admin session; revisit if the app adopts CSRF tokens app-wide. |
| Plaintext password in the request body / logs | **HTTPS required in production** (see Key Technical Decisions): the `Secure` cookie + TLS in transit both depend on `BaseURL` being https; over http the password and cookie are exposed. Handler must never log the password/confirm/hash/body — only ids + outcome. |
| One admin can reset another admin's password (lateral takeover), with no audit trail | Accepted: the app trusts all admins equally and has no audit infra (out of scope per origin). `adminMiddleware` still gates the endpoint. Revisit if admin-role separation or audit logging is introduced. |
| Oversized request body / overlong password exhausts memory or feeds unbounded input to bcrypt | `http.MaxBytesReader(w, r.Body, 4096)` caps the body and a server-side 72-byte max-length bounds the hashed input (Unit 2); `maxlength="72"` mirrors it client-side (Unit 3). |
| Non-transactional hash-update + revocation could partially apply | Low impact (new password still valid); documented. Could wrap in a transaction later if desired. |
| `UpdateUserPassword` no-ops silently on a bad id (or a target deleted mid-request) | Prevented on the happy path by `GetUserByID`/`BobotUserID` guards before the update. Residual TOCTOU window (target deleted between check and update) accepted — see the R9 amendment in Key Technical Decisions. |

## Documentation / Operational Notes

- After implementation, consider a `docs/solutions/architecture-patterns/` write-up on "force re-login via `session_revocations` + reissue-path timing caveat" and "first admin mutation handler shape" — flagged as missing by the learnings search.
- New i18n keys must be added for every supported language in the `i18n/` catalog.
- This endpoint transmits a cleartext password; it must only be served over HTTPS (`BaseURL=https`, so the session cookie also gets `Secure`). Flag this as a hard requirement in deployment docs.

## Sources & References

- **Origin document:** `docs/brainstorms/2026-05-26-admin-change-user-password-requirements.md`
- Route/middleware: `server/server.go:152-155,174-176,207-291`
- Admin handler/guards: `server/admin.go:53-70`
- Settings writer: `server/pages.go:697-717`; logout revocation: `server/pages.go:370`
- DB methods: `db/core.go:1401-1407,1516-1532`
- Password helpers: `auth/password.go`; length check: `server/signup.go:173-179`
- Templates/JS: `web/templates/admin_user.html`, `web/templates/signup.html:5-25`, `web/static/settings.js:18-96`
- Tests: `server/server_test.go:202,460-464,503`, `server/push_test.go:12-29`, `db/core_test.go:313-329`
