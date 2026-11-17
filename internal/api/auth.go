package api

import (
	"net/http"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeJSON[credentials](w, r, 8<<10)
	if !ok {
		return
	}
	result, err := h.auth.Register(r.Context(), input.Email, input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeJSON[credentials](w, r, 8<<10)
	if !ok {
		return
	}
	result, err := h.auth.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.UserByID(r.Context(), owner(r))
	if err != nil {
		writeError(w, err)
		return
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, user)
}
