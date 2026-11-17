package api

import (
	"net/http"

	"github.com/Emulisy/Supa-Agent/internal/model"
	"github.com/Emulisy/Supa-Agent/internal/service"
)

func (h *handler) startRun(w http.ResponseWriter, r *http.Request) {
	id, ok := agentID(w, r)
	if !ok {
		return
	}
	input, ok := decodeJSON[service.ChatInput](w, r, 64<<10)
	if !ok {
		return
	}
	run, err := h.chat.Start(r.Context(), owner(r), id, *input)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/runs/"+run.ID)
	writeJSON(w, http.StatusAccepted, run)
}

func (h *handler) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.chat.GetRun(r.Context(), owner(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *handler) cancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.chat.CancelRun(r.Context(), owner(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if run.Status == model.RunRunning {
		status = http.StatusAccepted
	}
	writeJSON(w, status, run)
}

func (h *handler) listSessions(w http.ResponseWriter, r *http.Request) {
	list, err := h.chat.ListSessions(r.Context(), owner(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *handler) getSession(w http.ResponseWriter, r *http.Request) {
	session, err := h.chat.GetSession(r.Context(), owner(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (h *handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := h.chat.DeleteSession(r.Context(), owner(r), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
