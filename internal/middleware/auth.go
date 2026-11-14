package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Emulisy/Supa-Agent/internal/auth"
)

// A private key type prevents collisions with other request context values.
type userIDKey struct{}

// RequireAuth protects a handler with stateless bearer-JWT authentication.
func RequireAuth(s *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s == nil || next == nil {
				writeAuthError(w, http.StatusInternalServerError)
				return
			}
			values := r.Header.Values("Authorization")
			if len(values) != 1 {
				writeAuthError(w, http.StatusUnauthorized)
				return
			}
			parts := strings.Fields(values[0])
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeAuthError(w, http.StatusUnauthorized)
				return
			}
			userID, err := s.ValidateJWT(parts[1])
			if err != nil {
				if errors.Is(err, auth.ErrInvalidToken) {
					writeAuthError(w, http.StatusUnauthorized)
				} else {
					writeAuthError(w, http.StatusInternalServerError)
				}
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext returns the verified ID set by RequireAuth, if present.
func UserIDFromContext(ctx context.Context) (uint64, bool) {
	if ctx == nil {
		return 0, false
	}
	userID, ok := ctx.Value(userIDKey{}).(uint64)
	return userID, ok && userID != 0
}

func writeAuthError(w http.ResponseWriter, status int) {
	message := "internal server error"
	if status == http.StatusUnauthorized {
		message = "unauthorized"
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
