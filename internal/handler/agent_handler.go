package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/Emulisy/Supa-Agent/internal/agent"
	"github.com/Emulisy/Supa-Agent/internal/auth"
	"github.com/Emulisy/Supa-Agent/internal/llm"
	"github.com/Emulisy/Supa-Agent/internal/middleware"
)

const maxAgentRequestBytes = 64 << 10

type agentWriteRequest struct {
	Type   agent.Type      `json:"type"`
	Config json.RawMessage `json:"config"`
}

type agentResponse struct {
	ID     uint64          `json:"id"`
	UserID uint64          `json:"user_id"`
	Type   agent.Type      `json:"type"`
	Config json.RawMessage `json:"config"`
}

// CreateAgentHandler handles POST /agents. Authentication is included.
// Register with mux.Handle("POST /agents", CreateAgentHandler(service, authService)).
// The JSON body contains type and config; ownership always comes from the JWT.
func CreateAgentHandler(service *agent.Service, authService *auth.Service) http.HandlerFunc {
	protected := middleware.RequireAuth(authService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := agentRequestOwner(w, r, http.MethodPost, service)
		if !ok {
			return
		}
		input, ok := readAgentRequest(w, r)
		if !ok {
			return
		}
		definition := &agent.Definition{UserID: userID, Type: input.Type, Config: input.Config}
		if err := service.Save(r.Context(), definition); err != nil {
			writeAgentError(w, err)
			return
		}
		w.Header().Set("Location", "/agents/"+strconv.FormatUint(definition.ID, 10))
		writeJSON(w, http.StatusCreated, agentResponseFrom(definition))
	}))
	return protected.ServeHTTP
}

// UpdateAgentHandler handles PUT /agents/{id}. Authentication is included.
// Register with mux.Handle("PUT /agents/{id}", UpdateAgentHandler(service, authService)).
// PUT replaces type and config; ID and ownership cannot be supplied in the body.
func UpdateAgentHandler(service *agent.Service, authService *auth.Service) http.HandlerFunc {
	protected := middleware.RequireAuth(authService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := agentRequestOwner(w, r, http.MethodPut, service)
		if !ok {
			return
		}
		id, ok := readAgentID(w, r)
		if !ok {
			return
		}
		input, ok := readAgentRequest(w, r)
		if !ok {
			return
		}
		definition, err := service.FindByID(r.Context(), userID, id)
		if err != nil {
			writeAgentError(w, err)
			return
		}
		definition.Type = input.Type
		definition.Config = input.Config
		if err := service.Update(r.Context(), definition); err != nil {
			writeAgentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, agentResponseFrom(definition))
	}))
	return protected.ServeHTTP
}

// DeleteAgentHandler handles DELETE /agents/{id}. Authentication is included.
// Register with mux.Handle("DELETE /agents/{id}", DeleteAgentHandler(service, authService)).
// Configuration validation is unnecessary for deleting an owned definition.
func DeleteAgentHandler(service *agent.Service, authService *auth.Service) http.HandlerFunc {
	protected := middleware.RequireAuth(authService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := agentRequestOwner(w, r, http.MethodDelete, service)
		if !ok {
			return
		}
		id, ok := readAgentID(w, r)
		if !ok {
			return
		}
		if err := service.Delete(r.Context(), userID, id); err != nil {
			writeAgentError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	}))
	return protected.ServeHTTP
}

func agentRequestOwner(w http.ResponseWriter, r *http.Request, method string, service *agent.Service) (uint64, bool) {
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return 0, false
	}
	if service == nil {
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return 0, false
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return 0, false
	}
	return userID, true
}

func readAgentID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid agent ID")
		return 0, false
	}
	return id, true
}

func readAgentRequest(w http.ResponseWriter, r *http.Request) (*agentWriteRequest, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return nil, false
	}
	if r.ContentLength > maxAgentRequestBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input *agentWriteRequest
	if err := decoder.Decode(&input); err != nil {
		writeRequestBodyError(w, err)
		return nil, false
	}
	if input == nil {
		writeRequestBodyError(w, nil)
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeRequestBodyError(w, err)
		return nil, false
	}
	return input, true
}

func agentResponseFrom(definition *agent.Definition) agentResponse {
	return agentResponse{ID: definition.ID, UserID: definition.UserID, Type: definition.Type, Config: definition.Config}
}

func writeAgentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agent.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "agent not found")
	case errors.Is(err, agent.ErrInvalidDefinition):
		writeJSONError(w, http.StatusBadRequest, "invalid agent definition")
	case errors.Is(err, llm.ErrUnsupportedModel):
		writeJSONError(w, http.StatusBadRequest, "unsupported model")
	case errors.Is(err, context.DeadlineExceeded):
		writeJSONError(w, http.StatusGatewayTimeout, "request timed out")
	case errors.Is(err, context.Canceled):
		// The client has disconnected; avoid attempting another response write.
		return
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
	}
}
