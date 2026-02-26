package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalidSession = errors.New("invalid session token")
)

type SessionToken struct {
	UserID    int64     `json:"user_id"`
	Role      string    `json:"role"`
	Language  string    `json:"language,omitempty"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionService struct {
	key              []byte
	duration         time.Duration
	maxAge           time.Duration
	refreshThreshold time.Duration
}

func NewSessionService(secret string, duration, maxAge, refreshThreshold time.Duration) *SessionService {
	// Derive 32-byte key from secret using SHA-256
	hash := sha256.Sum256([]byte(secret))
	return &SessionService{
		key:              hash[:],
		duration:         duration,
		maxAge:           maxAge,
		refreshThreshold: refreshThreshold,
	}
}

func (s *SessionService) CreateToken(userID int64, role string, lang ...string) (string, error) {
	now := time.Now()
	token := &SessionToken{
		UserID:    userID,
		Role:      role,
		IssuedAt:  now,
		ExpiresAt: now.Add(s.duration),
	}
	if len(lang) > 0 {
		token.Language = lang[0]
	}

	plaintext, err := json.Marshal(token)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func (s *SessionService) DecryptToken(encrypted string) (*SessionToken, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, ErrInvalidSession
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, ErrInvalidSession
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidSession
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, ErrInvalidSession
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidSession
	}

	var token SessionToken
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return nil, ErrInvalidSession
	}

	return &token, nil
}

func (s *SessionService) NeedsReissue(token *SessionToken) bool {
	now := time.Now()
	// Needs reissue if expired OR within refresh threshold
	return now.After(token.ExpiresAt) || token.ExpiresAt.Sub(now) < s.refreshThreshold
}

func (s *SessionService) IsPastDeadline(token *SessionToken) bool {
	deadline := token.IssuedAt.Add(s.maxAge)
	return time.Now().After(deadline)
}

func (s *SessionService) Duration() time.Duration {
	return s.duration
}

func (s *SessionService) MaxAge() time.Duration {
	return s.maxAge
}
