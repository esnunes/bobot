---
date: 2026-05-26
topic: admin-change-user-password
---

# Admin: Change a User's Password

## Problem Frame
Today there is no way to change a user's password from within the app. Passwords
are only ever set at signup (`server/signup.go`) or via the admin-creation CLI
(`create_admin.go`), and `db/core.go` has no password-update method. If a user
forgets their password or an account needs to be secured, an admin's only
recourse is direct database manipulation or recreating the account.

This feature gives admins a first-class action — on the existing user detail page
(`GET /admin/users/{id}`, `web/templates/admin_user.html`) — to set a new
password for any user, closing that operational gap.

## Requirements

**UI (user detail page)**
- R1. Add a "Change password" section/form to `web/templates/admin_user.html`, shown on the user detail page. The page is already gated to admins by `adminMiddleware`.
- R2. The form has a **new password** field and a **confirm password** field. The admin types the password directly.
- R3. On success, show a clear inline confirmation. On failure (mismatch, too short, server error), show the specific reason without changing the password.
- R10. Submitting requires a confirmation step that names the target user (by display name) and warns that they will be signed out of their sessions. The password changes only after the admin confirms.

**Behavior & backend**
- R4. A new admin-only endpoint changes the target user's password, enforced by `adminMiddleware` (non-admins get 403).
- R5. The two password fields must match; mismatched submissions are rejected with a clear error and no change.
- R6. The new password must meet the same minimum length as signup (8 characters). Shorter passwords are rejected with a clear error.
- R7. The password is hashed via `auth.HashPassword` and persisted via a new `UpdateUserPassword(userID, passwordHash)` DB method modeled on `UpdateUserEmail`/`UpdateUserDisplayName` in `db/core.go`.
- R8. On a successful change, revoke the target user's existing sessions via `CreateSessionRevocation(userID, reason)`. Revocation is **eventual** by design: it takes effect when the user's token next refreshes (up to ~one session duration, default 30m), and an open chat WebSocket persists until it disconnects. Immediate cut-off and closing live connections are out of scope (see Scope Boundaries).
- R9. The endpoint validates the target user before reporting success: return 404 for an unknown user ID or the reserved bobot system user (mirroring `handleAdminUserPage`), and treat an UPDATE that affects zero rows as an error, not a success.

## Success Criteria
- An admin can set a new password for any user from the user detail page, and that user can immediately log in with the new password.
- After a successful change, the target user's sessions are revoked and stop working at their next token refresh (up to ~one session duration), at which point re-login with the new password is required. (Immediate cut-off and closing live connections are out of scope.)
- Before the change is applied, the admin sees a confirmation naming the target user and warning about sign-out.
- Mismatched or too-short passwords are rejected with a clear message and leave the existing password unchanged.
- A non-admin cannot reach the endpoint (403).

## Scope Boundaries
- **Self-service password change** for regular users (changing their own password in `/settings`) is out of scope — this feature is admin-initiated only.
- **Forgot-password / email reset flow** is out of scope; there is no email-sending infrastructure in the repo.
- **Force-change-on-next-login** is out of scope; no such infrastructure exists. The user simply logs in with the password the admin set.
- **Audit logging** of the action is out of scope as a feature; no audit infrastructure exists today. A plain `slog` log line in the handler is acceptable — but it (and any error output) must never include the password, confirm-password, password hash, or request body; log only the acting admin ID, target user ID, and outcome.
- **Generated/temporary passwords** and password-complexity rules beyond minimum length are out of scope (decided: admin types the password).
- **Immediate session cut-off** (checking revocation on every authenticated request) and **closing live WebSocket connections** on reset are out of scope; revocation is eventual (see Key Decisions).

## Key Decisions
- **Admin types the password directly** (with a confirm field) rather than generating a temporary one: simplest, matches how passwords are set today, and there is no email infra to deliver a generated secret anyway.
- **Revoke the target user's sessions on change**: security best practice for a reset. Caveat surfaced in review: the existing `session_revocations` check is **not** immediate (see Dependencies), so whether logout must be immediate is an open decision — not the settled "low-cost" win first assumed.
- **Reuse signup's minimum-length rule (8 chars)** rather than introducing a new policy, for consistency.
- **Revocation is eventual, not immediate** (decided after review): logout takes effect at the user's next token refresh (~up to session duration) and live chats persist until disconnect. Immediate cut-off was considered and rejected — not worth a per-request DB check plus connection-teardown logic for a low-frequency admin action.
- **A confirmation step is required before applying** (decided after review): it names the target user and warns about sign-out, guarding against accidental resets or lockouts.
- **An admin resetting their own password through this form is allowed.** Because revocation is eventual, the admin keeps their current session through the refresh window (so R3's inline confirmation renders normally) and is logged out at the next refresh. No special-case handling needed.

## Dependencies / Assumptions
- Signup currently enforces an 8-character minimum (`server/signup.go`); R6 reuses that value.
- The `session_revocations` mechanism exists, but verification of the actual behavior shows revocation is **not** immediate: `HasSessionRevocation` is only checked inside the `NeedsReissue` branch of `sessionMiddleware`, i.e. when a token is at/near expiry. An already-issued token keeps authenticating until its refresh window — up to roughly one session duration (default 30m). Additionally, live WebSocket chat connections (`handleChat`) authenticate once at upgrade and never re-check, so they persist until the socket drops. R8 and the second success criterion depend on resolving how immediate logout must be.

## Outstanding Questions

### Deferred to Planning
- [Affects R3][Technical] `admin_user.html` is currently read-only (info rows + collapsible `<details>`); it has no form or inline-feedback pattern, and this is the first admin-side mutation endpoint. Establish the form + inline error/success pattern (HTMX partial swap vs. full POST), using `settings.js` feedback and `signup.html` only as loose analogs.
- [Affects R5/R3][Technical] Where mismatch and length are validated (client-side, server-side, or both) and how errors surface inline; keep server-side as source of truth.
- [Affects R3][Technical] Submit/loading state (disable the button to prevent double-submit), clearing fields on success, and field handling on the error path.
- [Affects R2][Technical] Accessible field labels (not placeholder-only), `autocomplete="new-password"` to stop the browser autofilling the admin's own credentials, `minlength=8`, and i18n keys for all new strings.
- [Affects R1][Technical] Placement/grouping of the section within `admin_user.html` (e.g. a distinct "Account actions" group, visually separated from read-only data).
- [Affects R6][Technical] No shared minimum-length constant exists; `validatePassword` hardcodes `< 8`. Decide whether to extract a shared constant/validator or accept duplicating the literal.
- [Affects R4][Security] CSRF posture: the app currently relies solely on `SameSite=Lax` cookies (no CSRF tokens anywhere). Decide whether this new mutation inherits that (document the accepted risk) or adds a token.
- [Affects R4][Security] Step-up re-auth: decide whether the admin must re-enter their own password before resetting another user's. Default: not required (admin session is sufficient); document the accepted risk.
- [Affects R8/Success Criteria][Technical] Behavior for a blocked target user — password is set but login remains blocked; clarify the success-criterion wording accordingly.

## Next Steps
→ `/ce:plan` for structured implementation planning
