package api

import (
	"errors"
	"net/http"

	"github.com/Emulisy/Supa-Agent/internal/service"
)

type handler struct {
	auth   *service.AuthService
	agents *service.AgentService
	chat   *service.ChatService
}

func NewRouter(auth *service.AuthService, agents *service.AgentService, chat *service.ChatService, tokens TokenVerifier) (http.Handler, error) {
	if auth == nil || agents == nil || chat == nil || tokens == nil {
		return nil, errors.New("api: all dependencies are required")
	}
	h := &handler{auth: auth, agents: agents, chat: chat}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /users", h.register)
	mux.HandleFunc("POST /auth/tokens", h.login)
	protect := func(pattern string, next http.HandlerFunc) { mux.Handle(pattern, requireAuth(tokens, next)) }
	protect("GET /users/me", h.me)
	protect("POST /agents", h.createAgent)
	protect("PUT /agents/{id}", h.updateAgent)
	protect("DELETE /agents/{id}", h.deleteAgent)
	protect("GET /agents", h.listAgents)
	protect("GET /agents/{id}", h.getAgent)
	protect("POST /agents/{id}/runs", h.startRun)
	protect("GET /runs/{id}", h.getRun)
	protect("POST /runs/{id}/cancel", h.cancelRun)
	protect("GET /runs/{id}/events", h.runEvents)
	protect("GET /sessions", h.listSessions)
	protect("GET /sessions/{id}", h.getSession)
	protect("DELETE /sessions/{id}", h.deleteSession)
	return mux, nil
}
