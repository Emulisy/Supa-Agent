package api

import (
	"net/http"
	"strconv"

	"github.com/Emulisy/Supa-Agent/internal/model"
)

func (h *handler) listMemories(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentID(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	items, err := h.memories.List(r.Context(), model.MemoryScope{UserID: owner(r), AgentID: agent}, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) changeMemory(w http.ResponseWriter, r *http.Request) {
	agent, ok := agentID(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("memory_id"), 10, 64)
	if err != nil || id == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid memory ID")
		return
	}
	scope := model.MemoryScope{UserID: owner(r), AgentID: agent}
	if r.Method == http.MethodDelete {
		err = h.memories.Delete(r.Context(), scope, id)
	} else {
		err = h.memories.Archive(r.Context(), scope, id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
