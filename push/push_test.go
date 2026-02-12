package push

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/db"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	t.Helper()
	dir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("setup db: %v", err)
	}
	t.Cleanup(func() { coreDB.Close() })
	return coreDB
}

func generateVAPIDKeys(t *testing.T) (pubB64, privB64 string, privKey *ecdsa.PrivateKey) {
	t.Helper()
	ecdhKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubBytes := ecdhKey.PublicKey().Bytes() // 65-byte uncompressed
	privBytes := ecdhKey.Bytes()            // 32-byte scalar

	// Also reconstruct ecdsa key for callers that need it
	key := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(pubBytes[1:33]),
			Y:     new(big.Int).SetBytes(pubBytes[33:65]),
		},
		D: new(big.Int).SetBytes(privBytes),
	}
	return b64.EncodeToString(pubBytes), b64.EncodeToString(privBytes), key
}

func TestNewPushSender(t *testing.T) {
	coreDB := setupTestDB(t)
	pubB64, privB64, _ := generateVAPIDKeys(t)

	ps, err := NewPushSender(coreDB, pubB64, privB64, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewPushSender: %v", err)
	}
	if ps == nil {
		t.Fatal("expected non-nil PushSender")
	}
}

func TestNewPushSender_InvalidKeys(t *testing.T) {
	coreDB := setupTestDB(t)

	// Invalid public key
	_, err := NewPushSender(coreDB, "invalid", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "mailto:test@example.com")
	if err == nil {
		t.Fatal("expected error for invalid public key")
	}

	// Wrong length public key
	_, err = NewPushSender(coreDB, b64.EncodeToString([]byte("short")), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "mailto:test@example.com")
	if err == nil {
		t.Fatal("expected error for wrong length public key")
	}
}

func TestVAPIDJWT(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	jwt, err := signVAPIDJWT(key, "https://push.example.com", "mailto:test@example.com")
	if err != nil {
		t.Fatalf("signVAPIDJWT: %v", err)
	}

	// JWT should have 3 parts
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	// Verify header
	headerBytes, err := b64.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["typ"] != "JWT" || header["alg"] != "ES256" {
		t.Fatalf("unexpected header: %v", header)
	}

	// Verify claims
	claimsBytes, err := b64.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["aud"] != "https://push.example.com" {
		t.Fatalf("unexpected aud: %v", claims["aud"])
	}
	if claims["sub"] != "mailto:test@example.com" {
		t.Fatalf("unexpected sub: %v", claims["sub"])
	}

	// Verify signature
	sigBytes, err := b64.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if len(sigBytes) != 64 {
		t.Fatalf("expected 64-byte signature, got %d", len(sigBytes))
	}

	signingInput := parts[0] + "." + parts[1]
	hash := sha256.Sum256([]byte(signingInput))
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])
	if !ecdsa.Verify(&key.PublicKey, hash[:], r, s) {
		t.Fatal("signature verification failed")
	}
}

func TestEncrypt(t *testing.T) {
	// Generate subscriber keys
	subPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	subPubBytes := subPriv.PublicKey().Bytes()

	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatal(err)
	}

	p256dhB64 := b64.EncodeToString(subPubBytes)
	authB64 := b64.EncodeToString(authSecret)

	plaintext := []byte(`{"title":"Test","body":"Hello"}`)

	ciphertext, err := encrypt(p256dhB64, authB64, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Verify structure: salt(16) + rs(4) + idlen(1) + keyid(65) + ciphertext
	if len(ciphertext) < 86 {
		t.Fatalf("ciphertext too short: %d bytes", len(ciphertext))
	}

	// Check idlen = 65
	idlen := ciphertext[20]
	if idlen != 65 {
		t.Fatalf("expected idlen=65, got %d", idlen)
	}

	// Total header: 16 + 4 + 1 + 65 = 86
	// Encrypted payload should be plaintext+1(padding byte) + 16(GCM tag)
	expectedCiphertextLen := len(plaintext) + 1 + 16
	actualCiphertextLen := len(ciphertext) - 86
	if actualCiphertextLen != expectedCiphertextLen {
		t.Fatalf("expected ciphertext body %d bytes, got %d", expectedCiphertextLen, actualCiphertextLen)
	}
}

func TestSend_Success(t *testing.T) {
	// Setup test push server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Encoding") != "aes128gcm" {
			t.Errorf("expected aes128gcm encoding, got %s", r.Header.Get("Content-Encoding"))
		}
		if r.Header.Get("TTL") != "86400" {
			t.Errorf("expected TTL 86400, got %s", r.Header.Get("TTL"))
		}
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "vapid t=") {
			t.Errorf("expected vapid auth header, got %s", authHeader)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	coreDB := setupTestDB(t)
	pubB64, privB64, _ := generateVAPIDKeys(t)
	ps, err := NewPushSender(coreDB, pubB64, privB64, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Generate subscriber keys
	subPriv, _ := ecdh.P256().GenerateKey(rand.Reader)
	authSecret := make([]byte, 16)
	rand.Read(authSecret)

	status, err := ps.Send(
		ts.URL,
		b64.EncodeToString(subPriv.PublicKey().Bytes()),
		b64.EncodeToString(authSecret),
		[]byte(`{"title":"Test","body":"Hello"}`),
	)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d", status)
	}
}

func TestSend_Gone(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer ts.Close()

	coreDB := setupTestDB(t)
	pubB64, privB64, _ := generateVAPIDKeys(t)
	ps, err := NewPushSender(coreDB, pubB64, privB64, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	subPriv, _ := ecdh.P256().GenerateKey(rand.Reader)
	authSecret := make([]byte, 16)
	rand.Read(authSecret)

	status, err := ps.Send(
		ts.URL,
		b64.EncodeToString(subPriv.PublicKey().Bytes()),
		b64.EncodeToString(authSecret),
		[]byte(`{"title":"Test"}`),
	)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if status != http.StatusGone {
		t.Fatalf("expected 410, got %d", status)
	}
}

func TestNotifyUser_CleansUpStale(t *testing.T) {
	// One endpoint returns 410, the other 201
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusGone) // stale
		} else {
			w.WriteHeader(http.StatusCreated) // ok
		}
	}))
	defer ts.Close()

	coreDB := setupTestDB(t)

	// Create a test user
	user, err := coreDB.CreateUser("testuser", "hash")
	if err != nil {
		t.Fatal(err)
	}

	// Generate subscriber keys for two subscriptions
	subPriv1, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth1 := make([]byte, 16)
	rand.Read(auth1)

	subPriv2, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth2 := make([]byte, 16)
	rand.Read(auth2)

	// Save two subscriptions pointing to test server
	coreDB.SavePushSubscription(user.ID, ts.URL+"/sub1",
		b64.EncodeToString(subPriv1.PublicKey().Bytes()),
		b64.EncodeToString(auth1))
	coreDB.SavePushSubscription(user.ID, ts.URL+"/sub2",
		b64.EncodeToString(subPriv2.PublicKey().Bytes()),
		b64.EncodeToString(auth2))

	pubB64, privB64, _ := generateVAPIDKeys(t)
	ps, err := NewPushSender(coreDB, pubB64, privB64, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	ps.NotifyUser(user.ID, []byte(`{"title":"Test"}`))

	// Verify: first subscription should be deleted (410), second kept
	subs, err := coreDB.GetPushSubscriptions(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after cleanup, got %d", len(subs))
	}
	if subs[0].Endpoint != ts.URL+"/sub2" {
		t.Fatalf("expected second subscription to survive, got %s", subs[0].Endpoint)
	}
}

func TestBuildPayload(t *testing.T) {
	p := BuildPayload("Alice", "Hello!", "/chat", "msg-123")

	var data map[string]string
	if err := json.Unmarshal(p, &data); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if data["title"] != "Alice" {
		t.Fatalf("unexpected title: %s", data["title"])
	}
	if data["body"] != "Hello!" {
		t.Fatalf("unexpected body: %s", data["body"])
	}
	if data["url"] != "/chat" {
		t.Fatalf("unexpected url: %s", data["url"])
	}
	if data["tag"] != "msg-123" {
		t.Fatalf("unexpected tag: %s", data["tag"])
	}
}

func TestTruncateMessage(t *testing.T) {
	if got := TruncateMessage("short", 10); got != "short" {
		t.Fatalf("expected 'short', got '%s'", got)
	}
	if got := TruncateMessage("this is a long message", 10); got != "this is a ..." {
		t.Fatalf("expected truncated message, got '%s'", got)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
