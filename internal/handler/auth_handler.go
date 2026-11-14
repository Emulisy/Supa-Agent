package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/Emulisy/Supa-Agent/internal/auth"
	"github.com/Emulisy/Supa-Agent/internal/middleware"
)

const maxAuthRequestBytes = 8 << 10

// CreateUserHandler returns a POST /users handler using a shared auth service.
func CreateUserHandler(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		email, password, ok := readCredentials(w, r)
		if !ok {
			return
		}
		result, err := service.Register(r.Context(), email, password)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrInvalidEmail):
				writeJSONError(w, http.StatusBadRequest, "invalid email")
			case errors.Is(err, auth.ErrInvalidPassword):
				writeJSONError(w, http.StatusBadRequest, "invalid password")
			case errors.Is(err, auth.ErrEmailExists):
				writeJSONError(w, http.StatusConflict, "email already exists")
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal server error")
			}
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

// LoginUserHandler returns a POST /auth/tokens handler that issues an access JWT.
func LoginUserHandler(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		email, password, ok := readCredentials(w, r)
		if !ok {
			return
		}
		result, err := service.Login(r.Context(), email, password)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				writeJSONError(w, http.StatusUnauthorized, "invalid email or password")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "internal server error")
			}
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// GetMeHandler returns an authenticated GET /users/me handler.
// Authentication is included; callers can register this handler directly.
func GetMeHandler(service *auth.Service) http.HandlerFunc {
	protected := middleware.RequireAuth(service)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, ok := middleware.UserIDFromContext(r.Context())
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		u, err := service.UserByID(r.Context(), userID)
		if err != nil {
			if errors.Is(err, auth.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "user not found")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "internal server error")
			}
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			return
		}
		writeJSON(w, http.StatusOK, u)
	}))
	return protected.ServeHTTP
}

// readCredentials shares HTTP input handling between registration and login.
func readCredentials(w http.ResponseWriter, r *http.Request) (email, password string, ok bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return "", "", false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return "", "", false
	}
	if r.ContentLength > maxAuthRequestBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return "", "", false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input *struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decoder.Decode(&input); err != nil {
		writeRequestBodyError(w, err)
		return "", "", false
	}
	if input == nil {
		writeRequestBodyError(w, nil)
		return "", "", false
	}
	// A second decode must reach EOF; otherwise the body contains trailing data.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeRequestBodyError(w, err)
		return "", "", false
	}
	return input.Email, input.Password, true
}

func writeRequestBodyError(w http.ResponseWriter, err error) {
	var sizeErr *http.MaxBytesError
	if errors.As(err, &sizeErr) {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeJSONError(w, http.StatusBadRequest, "invalid JSON request body")
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	// Encode before committing headers so encoding failures can still return 500.
	body, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(`{"error":"internal server error"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}
