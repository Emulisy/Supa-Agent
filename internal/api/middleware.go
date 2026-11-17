package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Emulisy/Supa-Agent/internal/security"
)

type TokenVerifier interface{ Validate(string) (uint64, error) }
type userIDKey struct{}

func requireAuth(tokens TokenVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			unauthorized(w)
			return
		}
		parts := strings.Fields(values[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			unauthorized(w)
			return
		}
		id, err := tokens.Validate(parts[1])
		if errors.Is(err, security.ErrInvalidToken) || err == nil && id == 0 {
			unauthorized(w)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, id)))
	})
}

func owner(r *http.Request) uint64 { id, _ := r.Context().Value(userIDKey{}).(uint64); return id }
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeJSONError(w, http.StatusUnauthorized, "unauthorized")
}
