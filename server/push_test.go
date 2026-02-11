package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
)

func TestHandlePushSubscribe_Valid(t *testing.T) {
	s := setupTestServer(t)

	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	body := `{"endpoint":"https://push.example.com/v1/subscription/abc","keys":{"p256dh":"BNcRdre...","auth":"tBHI..."}}`
	req := httptest.NewRequest("POST", "/api/push/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Status = %d, want 201, body: %s", rr.Code, rr.Body.String())
	}

	// Verify subscription was saved
	subs, err := s.db.GetPushSubscriptions(user.ID)
	if err != nil {
		t.Fatalf("GetPushSubscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].Endpoint != "https://push.example.com/v1/subscription/abc" {
		t.Errorf("unexpected endpoint: %s", subs[0].Endpoint)
	}
}

func TestHandlePushSubscribe_MissingFields(t *testing.T) {
	s := setupTestServer(t)

	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	body := `{"endpoint":"https://push.example.com/v1/sub"}`
	req := httptest.NewRequest("POST", "/api/push/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400, body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePushSubscribe_NonHTTPS(t *testing.T) {
	s := setupTestServer(t)

	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	body := `{"endpoint":"http://push.example.com/v1/sub","keys":{"p256dh":"abc","auth":"def"}}`
	req := httptest.NewRequest("POST", "/api/push/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400, body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePushSubscribe_Unauthenticated(t *testing.T) {
	s := setupTestServer(t)

	body := `{"endpoint":"https://push.example.com/v1/sub","keys":{"p256dh":"abc","auth":"def"}}`
	req := httptest.NewRequest("POST", "/api/push/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", rr.Code)
	}
}

func TestHandlePushUnsubscribe(t *testing.T) {
	s := setupTestServer(t)

	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	// First subscribe
	s.db.SavePushSubscription(user.ID, "https://push.example.com/sub1", "p256dh", "auth")

	// Then unsubscribe
	body := `{"endpoint":"https://push.example.com/sub1"}`
	req := httptest.NewRequest("DELETE", "/api/push/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want 204, body: %s", rr.Code, rr.Body.String())
	}

	// Verify subscription was deleted
	subs, _ := s.db.GetPushSubscriptions(user.ID)
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions, got %d", len(subs))
	}
}

func TestHandlePushUnsubscribe_MissingEndpoint(t *testing.T) {
	s := setupTestServer(t)

	hash, _ := auth.HashPassword("password123")
	user, _ := s.db.CreateUserFull("testuser", hash, "Test", "user")
	token, _ := s.session.CreateToken(user.ID, "user")

	body := `{}`
	req := httptest.NewRequest("DELETE", "/api/push/subscribe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400, body: %s", rr.Code, rr.Body.String())
	}
}
