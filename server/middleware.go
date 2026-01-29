// server/middleware.go
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type contextKey string

const userIDKey contextKey = "user_id"

type AuthMiddleware struct {
	jwt *auth.JWTService
	db  *db.CoreDB
}

func NewAuthMiddleware(jwt *auth.JWTService, db *db.CoreDB) *AuthMiddleware {
	return &AuthMiddleware{jwt: jwt, db: db}
}

func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		claims, err := m.jwt.ValidateAccessToken(parts[1])
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserID(ctx context.Context) int64 {
	if id, ok := ctx.Value(userIDKey).(int64); ok {
		return id
	}
	return 0
}
