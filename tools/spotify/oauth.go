// tools/spotify/oauth.go
package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"crypto/rand"
	"encoding/base64"

	"golang.org/x/oauth2"
)

var spotifyEndpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.spotify.com/authorize",
	TokenURL: "https://accounts.spotify.com/api/token",
}

const spotifyScopes = "user-read-private user-read-playback-state user-modify-playback-state user-read-currently-playing playlist-read-private playlist-read-collaborative"

// SpotifyOAuth holds the OAuth2 configuration for Spotify.
type SpotifyOAuth struct {
	config *oauth2.Config
	db     *SpotifyDB
	mu     sync.Map // per-user mutexes for token refresh
}

// NewOAuth creates a new SpotifyOAuth from client credentials.
func NewOAuth(clientID, clientSecret, baseURL string, db *SpotifyDB) *SpotifyOAuth {
	return &SpotifyOAuth{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  baseURL + "/api/spotify/callback",
			Scopes:       strings.Split(spotifyScopes, " "),
			Endpoint:     spotifyEndpoint,
		},
		db: db,
	}
}

// GenerateAuthURL creates a Spotify OAuth2 authorization URL with CSRF state and PKCE.
func (o *SpotifyOAuth) GenerateAuthURL(userID, topicID int64) (string, error) {
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
		oauth2.SetAuthURLParam("show_dialog", "true"),
	)

	return authURL, nil
}

// ExchangeCode exchanges an authorization code for tokens.
// Returns (topicID, userID, token, error) so the caller can verify the session user
// and check Premium status before persisting.
func (o *SpotifyOAuth) ExchangeCode(ctx context.Context, code, state string) (int64, int64, *oauth2.Token, error) {
	oauthState, err := o.db.GetAndDeleteOAuthState(state)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("looking up state: %w", err)
	}
	if oauthState == nil {
		return 0, 0, nil, fmt.Errorf("invalid or expired OAuth state")
	}

	token, err := o.config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(oauthState.Verifier),
	)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("exchanging code: %w", err)
	}

	return oauthState.TopicID, oauthState.UserID, token, nil
}

// SaveTokenAndLink stores the OAuth token for the user and creates a topic link.
func (o *SpotifyOAuth) SaveTokenAndLink(userID, topicID int64, token *oauth2.Token) error {
	if err := o.db.SaveToken(TokenRecord{
		UserID:       userID,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
	}); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}

	if err := o.db.LinkTopic(topicID, userID); err != nil {
		return fmt.Errorf("linking topic: %w", err)
	}

	return nil
}

// GetTokenSource returns an oauth2.TokenSource for the given user.
// It handles automatic refresh and persists refreshed tokens.
// Returns nil if no token exists for the user.
func (o *SpotifyOAuth) GetTokenSource(userID int64) (oauth2.TokenSource, error) {
	record, err := o.db.GetToken(userID)
	if err != nil {
		return nil, fmt.Errorf("loading token: %w", err)
	}
	if record == nil {
		return nil, nil
	}

	token := &oauth2.Token{
		AccessToken:  record.AccessToken,
		RefreshToken: record.RefreshToken,
		TokenType:    record.TokenType,
		Expiry:       record.Expiry,
	}

	base := o.config.TokenSource(context.Background(), token)

	return &persistingTokenSource{
		base:    base,
		userID:  userID,
		db:      o.db,
		current: token,
		mu:      o.getUserMutex(userID),
	}, nil
}

// CheckPremium calls the Spotify /v1/me endpoint and checks if the user has Premium.
func CheckPremium(ctx context.Context, accessToken string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/me", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("calling /v1/me: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return false, fmt.Errorf("Spotify API error %d: %s", resp.StatusCode, body)
	}

	var user struct {
		Product string `json:"product"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return false, fmt.Errorf("decoding user response: %w", err)
	}

	return user.Product == "premium", nil
}

func (o *SpotifyOAuth) getUserMutex(userID int64) *sync.Mutex {
	val, _ := o.mu.LoadOrStore(userID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// persistingTokenSource wraps an oauth2.TokenSource and saves refreshed tokens to the DB.
type persistingTokenSource struct {
	base    oauth2.TokenSource
	userID  int64
	db      *SpotifyDB
	current *oauth2.Token
	mu      *sync.Mutex
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	token, err := p.base.Token()
	if err != nil {
		if isTokenRevoked(err) {
			p.db.Disconnect(p.userID)
			return nil, fmt.Errorf("Spotify access was revoked. Reconnect Spotify from topic settings.")
		}
		return nil, err
	}

	// Persist if token was refreshed
	if token.AccessToken != p.current.AccessToken {
		if err := p.db.SaveToken(TokenRecord{
			UserID:       p.userID,
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			TokenType:    token.TokenType,
			Expiry:       token.Expiry,
		}); err != nil {
			slog.Error("spotify: failed to persist refreshed token", "user_id", p.userID, "error", err)
		}
		p.current = token
	}

	return token, nil
}

func isTokenRevoked(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	for _, sub := range []string{"invalid_grant", "token has been revoked", "token has been expired", "refresh token revoked"} {
		if strings.Contains(errStr, sub) {
			return true
		}
	}
	return false
}

