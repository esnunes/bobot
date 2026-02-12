package push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/esnunes/bobot/db"
	"golang.org/x/crypto/hkdf"
)

// base64url encoding without padding (per RFC 7515)
var b64 = base64.RawURLEncoding

// PushSender handles Web Push encryption and delivery.
type PushSender struct {
	db         *db.CoreDB
	vapidKey   *ecdsa.PrivateKey
	vapidPub   []byte // 65-byte uncompressed public key
	subject    string
	httpClient *http.Client
}

// NewPushSender creates a PushSender from base64url-encoded VAPID keys.
func NewPushSender(coreDB *db.CoreDB, publicKeyB64, privateKeyB64, subject string) (*PushSender, error) {
	pubBytes, err := b64.DecodeString(publicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode vapid public key: %w", err)
	}
	if len(pubBytes) != 65 {
		return nil, fmt.Errorf("vapid public key must be 65 bytes (uncompressed P-256), got %d", len(pubBytes))
	}

	privBytes, err := b64.DecodeString(privateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode vapid private key: %w", err)
	}
	if len(privBytes) != 32 {
		return nil, fmt.Errorf("vapid private key must be 32 bytes, got %d", len(privBytes))
	}

	// Validate public key by parsing via ecdh
	_, err = ecdh.P256().NewPublicKey(pubBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid vapid public key: %w", err)
	}

	// Reconstruct ecdsa.PrivateKey from raw scalar + public key coordinates
	// pubBytes[0] = 0x04, pubBytes[1:33] = X, pubBytes[33:65] = Y
	privKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(pubBytes[1:33]),
			Y:     new(big.Int).SetBytes(pubBytes[33:65]),
		},
		D: new(big.Int).SetBytes(privBytes),
	}

	return &PushSender{
		db:       coreDB,
		vapidKey: privKey,
		vapidPub: pubBytes,
		subject:  subject,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// NotifyUser sends a push notification to all subscriptions for a user.
// Stale subscriptions (404/410) are cleaned up automatically.
func (ps *PushSender) NotifyUser(userID int64, payload []byte) {
	slog.Debug("push: looking up subscriptions", "user_id", userID)

	subs, err := ps.db.GetPushSubscriptions(userID)
	if err != nil {
		slog.Error("push: failed to get subscriptions", "user_id", userID, "error", err)
		return
	}

	slog.Debug("push: found subscriptions", "user_id", userID, "count", len(subs))

	for _, sub := range subs {
		slog.Debug("push: sending to endpoint", "user_id", userID, "endpoint", sub.Endpoint)

		status, err := ps.Send(sub.Endpoint, sub.P256DH, sub.Auth, payload)
		if err != nil {
			slog.Error("push: send failed", "endpoint", sub.Endpoint, "error", err)
			continue
		}

		slog.Debug("push: endpoint responded", "endpoint", sub.Endpoint, "status", status)

		if status == http.StatusNotFound || status == http.StatusGone {
			slog.Info("push: removing stale subscription", "endpoint", sub.Endpoint, "status", status)
			ps.db.DeletePushSubscription(sub.Endpoint)
		} else if status >= 400 {
			slog.Warn("push: endpoint returned error", "endpoint", sub.Endpoint, "status", status)
		}
	}
}

// Send encrypts the payload and delivers it to a push service endpoint.
// Returns the HTTP status code and any error.
func (ps *PushSender) Send(endpoint, p256dhB64, authB64 string, payload []byte) (int, error) {
	slog.Debug("push: encrypting payload", "endpoint", endpoint, "payload_size", len(payload))

	body, err := encrypt(p256dhB64, authB64, payload)
	if err != nil {
		return 0, fmt.Errorf("encrypt: %w", err)
	}

	slog.Debug("push: generating VAPID auth", "endpoint", endpoint)

	authHeader, err := ps.vapidAuth(endpoint)
	if err != nil {
		return 0, fmt.Errorf("vapid auth: %w", err)
	}

	slog.Debug("push: posting to push service", "endpoint", endpoint, "body_size", len(body))

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")

	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()

	return resp.StatusCode, nil
}

// vapidAuth generates the VAPID Authorization header value.
func (ps *PushSender) vapidAuth(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	audience := u.Scheme + "://" + u.Host

	jwt, err := signVAPIDJWT(ps.vapidKey, audience, ps.subject)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("vapid t=%s,k=%s", jwt, b64.EncodeToString(ps.vapidPub)), nil
}

// signVAPIDJWT creates an ES256-signed JWT for VAPID (RFC 8292).
func signVAPIDJWT(key *ecdsa.PrivateKey, audience, subject string) (string, error) {
	header := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))

	claims, _ := json.Marshal(map[string]any{
		"aud": audience,
		"exp": time.Now().Add(12 * time.Hour).Unix(),
		"sub": subject,
	})
	payload := b64.EncodeToString(claims)

	signingInput := header + "." + payload
	hash := sha256.Sum256([]byte(signingInput))

	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", err
	}

	// ES256 signature: r and s as 32-byte big-endian integers concatenated
	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):], sBytes)

	return signingInput + "." + b64.EncodeToString(sig), nil
}

// encrypt implements Web Push payload encryption per RFC 8291 (aes128gcm).
func encrypt(p256dhB64, authB64 string, plaintext []byte) ([]byte, error) {
	// Decode subscriber keys
	uaPublicBytes, err := b64.DecodeString(p256dhB64)
	if err != nil {
		return nil, fmt.Errorf("decode p256dh: %w", err)
	}
	authSecret, err := b64.DecodeString(authB64)
	if err != nil {
		return nil, fmt.Errorf("decode auth: %w", err)
	}

	// Parse subscriber's public key via crypto/ecdh
	uaPub, err := ecdh.P256().NewPublicKey(uaPublicBytes)
	if err != nil {
		return nil, fmt.Errorf("parse ua public key: %w", err)
	}

	// Generate ephemeral key pair
	ephPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	ephPubBytes := ephPriv.PublicKey().Bytes()

	// ECDH shared secret
	sharedSecret, err := ephPriv.ECDH(uaPub)
	if err != nil {
		return nil, err
	}

	// Generate 16-byte random salt
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	// IKM = HKDF(auth_secret, shared_secret, "WebPush: info\0" || ua_pub || as_pub) → 32 bytes
	infoIKM := append([]byte("WebPush: info\x00"), uaPublicBytes...)
	infoIKM = append(infoIKM, ephPubBytes...)
	ikmReader := hkdf.New(sha256.New, sharedSecret, authSecret, infoIKM)
	ikm := make([]byte, 32)
	if _, err := io.ReadFull(ikmReader, ikm); err != nil {
		return nil, err
	}

	// CEK = HKDF(salt, IKM, "Content-Encoding: aes128gcm\0") → 16 bytes
	cekReader := hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: aes128gcm\x00"))
	cek := make([]byte, 16)
	if _, err := io.ReadFull(cekReader, cek); err != nil {
		return nil, err
	}

	// Nonce = HKDF(salt, IKM, "Content-Encoding: nonce\0") → 12 bytes
	nonceReader := hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: nonce\x00"))
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(nonceReader, nonce); err != nil {
		return nil, err
	}

	// Encrypt: AES-128-GCM(CEK, nonce, plaintext || 0x02)
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Pad plaintext with delimiter byte 0x02 (single record, last record)
	padded := append(plaintext, 0x02)
	ciphertext := gcm.Seal(nil, nonce, padded, nil)

	// Record size: content = 4096 (default for single record)
	rs := uint32(4096)

	// Build aes128gcm header: salt(16) || rs(4, BE) || idlen(1) || keyid(65) || ciphertext
	var buf bytes.Buffer
	buf.Write(salt)
	binary.Write(&buf, binary.BigEndian, rs)
	buf.WriteByte(byte(len(ephPubBytes)))
	buf.Write(ephPubBytes)
	buf.Write(ciphertext)

	return buf.Bytes(), nil
}

// BuildPayload creates a JSON push notification payload.
func BuildPayload(title, body, urlPath, tag string) []byte {
	p, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
		"url":   urlPath,
		"tag":   tag,
	})
	return p
}

// TruncateMessage returns the first maxLen characters of a string, adding "..." if truncated.
func TruncateMessage(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
