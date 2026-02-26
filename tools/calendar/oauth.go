// tools/calendar/oauth.go
package calendar

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcalendar "google.golang.org/api/calendar/v3"
)

// OAuthConfig holds the OAuth2 configuration for Google Calendar.
type OAuthConfig struct {
	config *oauth2.Config
	db     *CalendarDB
	mu     sync.Map // per-topic mutexes for token refresh
}

// NewOAuthConfig creates a new OAuthConfig from client credentials.
func NewOAuthConfig(clientID, clientSecret, baseURL string, db *CalendarDB) *OAuthConfig {
	return &OAuthConfig{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  baseURL + "/api/calendar/callback",
			Scopes: []string{
				gcalendar.CalendarEventsScope,
				gcalendar.CalendarCalendarlistReadonlyScope,
			},
			Endpoint: google.Endpoint,
		},
		db: db,
	}
}

// GenerateAuthURL creates an OAuth2 authorization URL with CSRF state and PKCE.
func (o *OAuthConfig) GenerateAuthURL(userID, topicID int64) (string, error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	verifier := oauth2.GenerateVerifier()

	if err := o.db.SaveOAuthState(OAuthState{
		State:    state,
		UserID:   userID,
		TopicID:  topicID,
		Verifier: verifier,
	}); err != nil {
		return "", fmt.Errorf("saving oauth state: %w", err)
	}

	authURL := o.config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	return authURL, nil
}

// ExchangeCode exchanges an authorization code for tokens and stores them.
// Returns (topicID, userID, error) so the caller can verify the session user.
func (o *OAuthConfig) ExchangeCode(ctx context.Context, code, state string) (int64, int64, error) {
	oauthState, err := o.db.GetAndDeleteOAuthState(state)
	if err != nil {
		return 0, 0, fmt.Errorf("looking up state: %w", err)
	}
	if oauthState == nil {
		return 0, 0, fmt.Errorf("invalid or expired OAuth state")
	}

	token, err := o.config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(oauthState.Verifier),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("exchanging code: %w", err)
	}

	if err := o.db.SaveToken(TokenRecord{
		TopicID:      oauthState.TopicID,
		UserID:       oauthState.UserID,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenExpiry:  token.Expiry,
	}); err != nil {
		return 0, 0, fmt.Errorf("saving token: %w", err)
	}

	return oauthState.TopicID, oauthState.UserID, nil
}

// GetTokenSource returns an oauth2.TokenSource for the given topic.
// It handles automatic refresh and persists refreshed tokens.
// Returns nil if no token exists for the topic.
func (o *OAuthConfig) GetTokenSource(topicID int64) (oauth2.TokenSource, error) {
	record, err := o.db.GetToken(topicID)
	if err != nil {
		return nil, fmt.Errorf("loading token: %w", err)
	}
	if record == nil {
		return nil, nil
	}

	token := &oauth2.Token{
		AccessToken:  record.AccessToken,
		RefreshToken: record.RefreshToken,
		TokenType:    "Bearer",
		Expiry:       record.TokenExpiry,
	}

	base := o.config.TokenSource(context.Background(), token)

	return &persistingTokenSource{
		base:    base,
		topicID: topicID,
		db:      o.db,
		current: token,
		mu:      o.getTopicMutex(topicID),
	}, nil
}

// RevokeToken sends a best-effort revocation request to Google.
// It does NOT remove local data; the caller should call DB().Disconnect() separately.
func (o *OAuthConfig) RevokeToken(topicID int64) error {
	record, err := o.db.GetToken(topicID)
	if err != nil {
		return fmt.Errorf("loading token: %w", err)
	}
	if record == nil {
		return nil
	}

	tokenToRevoke := record.RefreshToken
	if tokenToRevoke == "" {
		tokenToRevoke = record.AccessToken
	}
	resp, err := http.PostForm("https://oauth2.googleapis.com/revoke", url.Values{
		"token": {tokenToRevoke},
	})
	if err != nil {
		slog.Warn("calendar: revocation request failed", "error", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			slog.Warn("calendar: revocation returned non-200", "status", resp.StatusCode)
		}
	}

	return nil
}

func (o *OAuthConfig) getTopicMutex(topicID int64) *sync.Mutex {
	val, _ := o.mu.LoadOrStore(topicID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// persistingTokenSource wraps an oauth2.TokenSource and saves refreshed tokens to the DB.
type persistingTokenSource struct {
	base    oauth2.TokenSource
	topicID int64
	db      *CalendarDB
	current *oauth2.Token
	mu      *sync.Mutex
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	token, err := p.base.Token()
	if err != nil {
		// If refresh fails, check if it's a revocation
		if isTokenRevoked(err) {
			p.db.Disconnect(p.topicID)
			return nil, fmt.Errorf("Google Calendar access was revoked. The topic owner needs to reconnect from settings.")
		}
		return nil, err
	}

	// Persist if token was refreshed
	if token.AccessToken != p.current.AccessToken {
		if err := p.db.SaveToken(TokenRecord{
			TopicID:      p.topicID,
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			TokenExpiry:  token.Expiry,
		}); err != nil {
			slog.Error("calendar: failed to persist refreshed token", "topic_id", p.topicID, "error", err)
		}
		p.current = token
	}

	return token, nil
}

func isTokenRevoked(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Google returns "invalid_grant" when refresh token is revoked
	for _, sub := range []string{"invalid_grant", "Token has been revoked", "Token has been expired"} {
		if strings.Contains(errStr, sub) {
			return true
		}
	}
	return false
}
