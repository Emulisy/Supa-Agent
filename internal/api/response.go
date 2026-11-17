package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/Emulisy/Supa-Agent/internal/llm"
	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/Emulisy/Supa-Agent/internal/security"
)

func decodeJSON[T any](w http.ResponseWriter, r *http.Request, max int64) (*T, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return nil, false
	}
	if r.ContentLength > max {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var value *T
	if err := decoder.Decode(&value); err != nil {
		writeBodyError(w, err)
		return nil, false
	}
	if value == nil {
		writeBodyError(w, nil)
		return nil, false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeBodyError(w, err)
		return nil, false
	}
	return value, true
}

func writeBodyError(w http.ResponseWriter, err error) {
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeJSONError(w, http.StatusBadRequest, "invalid JSON request body")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		status, body = http.StatusInternalServerError, []byte(`{"error":"internal server error"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, model.ErrUserNotFound):
		status, message = http.StatusNotFound, "user not found"
	case errors.Is(err, model.ErrAgentNotFound):
		status, message = http.StatusNotFound, "agent not found"
	case errors.Is(err, model.ErrSessionNotFound):
		status, message = http.StatusNotFound, "session not found"
	case errors.Is(err, model.ErrRunNotFound):
		status, message = http.StatusNotFound, "run not found"
	case errors.Is(err, model.ErrInvalidEmail):
		status, message = http.StatusBadRequest, "invalid email"
	case errors.Is(err, security.ErrInvalidPassword):
		status, message = http.StatusBadRequest, "invalid password"
	case errors.Is(err, model.ErrInvalidCredentials):
		status, message = http.StatusUnauthorized, "invalid email or password"
	case errors.Is(err, model.ErrEmailExists):
		status, message = http.StatusConflict, "email already exists"
	case errors.Is(err, model.ErrInvalidDefinition):
		status, message = http.StatusBadRequest, "invalid agent definition"
	case errors.Is(err, llm.ErrUnsupportedModel):
		status, message = http.StatusBadRequest, "unsupported model"
	case errors.Is(err, model.ErrInvalidPagination):
		status, message = http.StatusBadRequest, "invalid pagination"
	case errors.Is(err, model.ErrInvalidInput):
		status, message = http.StatusBadRequest, "invalid request"
	case errors.Is(err, model.ErrConflict):
		status, message = http.StatusConflict, "session has an active run"
	case errors.Is(err, model.ErrCapacity):
		status, message = http.StatusTooManyRequests, "run capacity reached"
	case errors.Is(err, model.ErrUnavailable):
		status, message = http.StatusServiceUnavailable, "service unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		status, message = http.StatusGatewayTimeout, "request timed out"
	case errors.Is(err, context.Canceled):
		return
	}
	writeJSONError(w, status, message)
}
