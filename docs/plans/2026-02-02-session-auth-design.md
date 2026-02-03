# Session-Based Authentication Design

## Overview

Replace JWT-based authentication with a simpler encrypted session cookie approach, inspired by Rails' signed/encrypted cookies.

## Goals

- Simplify authentication logic (single token instead of access + refresh)
- Remove client-side token management
- Keep stateless operation for most requests
- Maintain ability to revoke sessions within a short window

## Token Design

### Structure

```go
type SessionToken struct {
    UserID    int64
    Role      string    // "admin" or "user"
    IssuedAt  time.Time
    ExpiresAt time.Time
}
```

### Encryption

- Algorithm: AES-256-GCM (authenticated encryption)
- Implementation: Go stdlib (`crypto/aes`, `crypto/cipher`)
- Format: `base64(nonce + ciphertext + tag)`
- Key derivation: SHA-256 hash of `BOBOT_JWT_SECRET` (or renamed env var)

### Cookie

- Name: `session`
- Flags: `HttpOnly`, `Secure` (HTTPS), `SameSite=Lax`, `Path=/`
- MaxAge: 7 days (browser expiry matches reissue window)

## Timing Configuration

| Setting | Default | Purpose |
|---------|---------|---------|
| `SESSION_DURATION` | 30 min | Token validity period |
| `SESSION_REFRESH_THRESHOLD` | 5 min | When to trigger early refresh |
| `SESSION_MAX_AGE` | 7 days | Maximum reissue window after expiry |

## Middleware Flow

```
1. Extract "session" cookie
   ↓ missing → redirect to login

2. Decrypt token
   ↓ fails → clear cookie, redirect to login

3. Check absolute deadline (IssuedAt + MaxAge)
   ↓ past 7 days → clear cookie, redirect to login

4. Check ExpiresAt
   ↓ valid and not near expiry → proceed (no DB hit)

5. [Expired OR near expiry] Database checks:
   a. user.blocked = true? → reject
   b. revocation record where revoked_at > token.IssuedAt? → reject

6. Reissue fresh token:
   - New IssuedAt = now
   - New ExpiresAt = now + SESSION_DURATION
   - Set updated cookie in response

7. Proceed with request
```

## Revocation

### Table Schema

```sql
CREATE TABLE session_revocations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    revoked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reason TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_revocations_user_revoked ON session_revocations(user_id, revoked_at);
```

### Revocation Check

```sql
SELECT 1 FROM session_revocations
WHERE user_id = ? AND revoked_at > ?
LIMIT 1
```

### When to Revoke

| Action | Mechanism |
|--------|-----------|
| Normal logout | Clear cookie only (no DB write) |
| Logout everywhere | Clear cookie + insert revocation record |
| Password change | Insert revocation record |
| Admin blocks user | Set `user.blocked = true` |

### Cleanup

Old revocation records (older than `SESSION_MAX_AGE`) can be periodically deleted.

## API Changes

### Modified Endpoints

| Endpoint | Change |
|----------|--------|
| `POST /` (login) | Set session cookie instead of JWT + refresh token |
| `POST /api/logout` | Clear cookie; add `?all=true` for logout everywhere |

### Removed Endpoints

| Endpoint | Reason |
|----------|--------|
| `POST /api/refresh` | Middleware handles reissue automatically |

## File Changes

### New Files

| File | Purpose |
|------|---------|
| `auth/session.go` | Token encrypt/decrypt, create/validate |

### Removed Files

| File | Reason |
|------|--------|
| `auth/jwt.go` | Replaced by session.go |
| `web/static/auth.js` | No client-side token management needed |

### Modified Files

| File | Changes |
|------|---------|
| `server/auth.go` | Login sets session cookie; logout with optional `?all=true` |
| `server/server.go` | Replace JWT middleware with session middleware |
| `server/chat.go` | WebSocket auth uses session token |
| `config/config.go` | Add session duration/max-age/threshold config |
| `db/core.go` | Add session_revocations table; remove refresh_tokens |
| `web/static/ws-manager.js` | Remove token refresh logic |

### Removed Database Table

- `refresh_tokens` - replaced by stateless reissue

## Session Service Interface

```go
type SessionService struct {
    encryptionKey []byte
    duration      time.Duration
    maxAge        time.Duration
    refreshThresh time.Duration
}

// Create new token (called at login)
func (s *SessionService) CreateToken(userID int64, role string) (string, error)

// Decrypt and parse token
func (s *SessionService) DecryptToken(encrypted string) (*SessionToken, error)

// Check if token needs reissue (expired or near expiry)
func (s *SessionService) NeedsReissue(token *SessionToken) bool

// Check if token is past absolute deadline
func (s *SessionService) IsPastDeadline(token *SessionToken) bool
```

## WebSocket Authentication

WebSocket connections authenticate once at upgrade time:

1. Read session cookie
2. Decrypt and validate token
3. Reject if expired past deadline
4. No reissue (one-time auth at connection)
5. Client handles reconnection on auth failure

## Security Properties

| Property | How it's achieved |
|----------|-------------------|
| Confidentiality | AES-256-GCM encryption |
| Integrity | GCM authentication tag |
| XSS protection | HttpOnly cookie |
| CSRF protection | SameSite=Lax |
| Session revocation | Database check on reissue (max 30 min delay) |
| Brute force | Encrypted token is opaque; no predictable structure |

## Migration

1. Deploy new code with session support
2. Existing JWT tokens will fail (users must re-login)
3. Drop `refresh_tokens` table after deployment
4. Remove `BOBOT_JWT_SECRET` env var (or rename to `BOBOT_SESSION_SECRET`)

## Future Enhancements

- Add `max_reissues` counter to limit indefinite session extension
- Add `absolute_expires_at` for hard session lifetime limit
- Add session generation number for instant "logout everywhere"
- Periodic cleanup job for old revocation records
