package spotify

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *SpotifyDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_spotify.db")
	db, err := NewSpotifyDB(dbPath)
	if err != nil {
		t.Fatalf("NewSpotifyDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOAuthState(t *testing.T) {
	db := newTestDB(t)

	state := OAuthState{
		State:    "test-state-123",
		UserID:   1,
		TopicID:  10,
		Verifier: "test-verifier",
	}

	if err := db.SaveOAuthState(state); err != nil {
		t.Fatalf("SaveOAuthState: %v", err)
	}

	got, err := db.GetAndDeleteOAuthState("test-state-123")
	if err != nil {
		t.Fatalf("GetAndDeleteOAuthState: %v", err)
	}
	if got == nil {
		t.Fatal("expected state, got nil")
	}
	if got.State != state.State || got.UserID != state.UserID || got.TopicID != state.TopicID || got.Verifier != state.Verifier {
		t.Errorf("state mismatch: got %+v, want %+v", got, state)
	}

	// Second call should return nil (state was deleted)
	got2, err := db.GetAndDeleteOAuthState("test-state-123")
	if err != nil {
		t.Fatalf("second GetAndDeleteOAuthState: %v", err)
	}
	if got2 != nil {
		t.Errorf("expected nil after deletion, got %+v", got2)
	}
}

func TestOAuthStateInvalid(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetAndDeleteOAuthState("nonexistent")
	if err != nil {
		t.Fatalf("GetAndDeleteOAuthState: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent state, got %+v", got)
	}
}

func TestTokenCRUD(t *testing.T) {
	db := newTestDB(t)

	token := TokenRecord{
		UserID:       1,
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}

	if err := db.SaveToken(token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	got, err := db.GetToken(1)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got == nil {
		t.Fatal("expected token, got nil")
	}
	if got.AccessToken != "access-123" || got.RefreshToken != "refresh-456" {
		t.Errorf("token mismatch: got %+v", got)
	}

	// HasToken
	has, err := db.HasToken(1)
	if err != nil {
		t.Fatalf("HasToken: %v", err)
	}
	if !has {
		t.Error("expected HasToken to return true")
	}

	has2, err := db.HasToken(999)
	if err != nil {
		t.Fatalf("HasToken(999): %v", err)
	}
	if has2 {
		t.Error("expected HasToken(999) to return false")
	}

	// Update token (upsert)
	token.AccessToken = "access-updated"
	if err := db.SaveToken(token); err != nil {
		t.Fatalf("SaveToken (update): %v", err)
	}
	got2, _ := db.GetToken(1)
	if got2.AccessToken != "access-updated" {
		t.Errorf("expected updated access token, got %s", got2.AccessToken)
	}

	// Delete
	if err := db.DeleteToken(1); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	got3, _ := db.GetToken(1)
	if got3 != nil {
		t.Error("expected nil after delete")
	}
}

func TestTopicLinks(t *testing.T) {
	db := newTestDB(t)

	// Must create token first (FK constraint)
	if err := db.SaveToken(TokenRecord{
		UserID:       1,
		AccessToken:  "a",
		RefreshToken: "r",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	// Link topics
	if err := db.LinkTopic(10, 1); err != nil {
		t.Fatalf("LinkTopic(10): %v", err)
	}
	if err := db.LinkTopic(20, 1); err != nil {
		t.Fatalf("LinkTopic(20): %v", err)
	}

	// Get link
	link, err := db.GetTopicLink(10)
	if err != nil {
		t.Fatalf("GetTopicLink: %v", err)
	}
	if link == nil || link.UserID != 1 {
		t.Errorf("expected link to user 1, got %+v", link)
	}

	// Nonexistent link
	link2, _ := db.GetTopicLink(999)
	if link2 != nil {
		t.Errorf("expected nil for nonexistent link, got %+v", link2)
	}

	// Get linked topics
	topics, err := db.GetLinkedTopics(1)
	if err != nil {
		t.Fatalf("GetLinkedTopics: %v", err)
	}
	if len(topics) != 2 {
		t.Errorf("expected 2 linked topics, got %d", len(topics))
	}

	// Unlink
	if err := db.UnlinkTopic(10); err != nil {
		t.Fatalf("UnlinkTopic: %v", err)
	}
	link3, _ := db.GetTopicLink(10)
	if link3 != nil {
		t.Error("expected nil after unlink")
	}

	topics2, _ := db.GetLinkedTopics(1)
	if len(topics2) != 1 {
		t.Errorf("expected 1 linked topic after unlink, got %d", len(topics2))
	}
}

func TestDisconnect(t *testing.T) {
	db := newTestDB(t)

	// Create token + links
	if err := db.SaveToken(TokenRecord{
		UserID:       1,
		AccessToken:  "a",
		RefreshToken: "r",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	db.LinkTopic(10, 1)
	db.LinkTopic(20, 1)

	// Disconnect
	if err := db.Disconnect(1); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	// Token should be gone
	token, _ := db.GetToken(1)
	if token != nil {
		t.Error("expected nil token after disconnect")
	}

	// Links should be gone
	topics, _ := db.GetLinkedTopics(1)
	if len(topics) != 0 {
		t.Errorf("expected 0 linked topics after disconnect, got %d", len(topics))
	}
}
