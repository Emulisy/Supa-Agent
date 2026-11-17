package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Emulisy/Supa-Agent/internal/model"
)

type agentWriteRequest struct {
	Type   model.Type      `json:"type"`
	Config json.RawMessage `json:"config"`
}

func agentID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid agent ID")
		return 0, false
	}
	return id, true
}

func (h *handler) createAgent(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeJSON[agentWriteRequest](w, r, 64<<10)
	if !ok {
		return
	}
	def := &model.Definition{UserID: owner(r), Type: input.Type, Config: input.Config}
	if err := h.agents.Save(r.Context(), def); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/agents/"+strconv.FormatUint(def.ID, 10))
	writeJSON(w, http.StatusCreated, def)
}

func (h *handler) updateAgent(w http.ResponseWriter, r *http.Request) {
	id, ok := agentID(w, r)
	if !ok {
		return
	}
	input, ok := decodeJSON[agentWriteRequest](w, r, 64<<10)
	if !ok {
		return
	}
	def, err := h.agents.FindByID(r.Context(), owner(r), id)
	if err != nil {
		writeError(w, err)
		return
	}
	def.Type, def.Config = input.Type, input.Config
	if err := h.agents.Update(r.Context(), def); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (h *handler) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id, ok := agentID(w, r)
	if !ok {
		return
	}
	if err := h.agents.Delete(r.Context(), owner(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) getAgent(w http.ResponseWriter, r *http.Request) {
	id, ok := agentID(w, r)
	if !ok {
		return
	}
	def, err := h.agents.FindByID(r.Context(), owner(r), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (h *handler) listAgents(w http.ResponseWriter, r *http.Request) {
	limit, offset := 20, 0
	var err error
	if value, ok := r.URL.Query()["limit"]; ok {
		if len(value) != 1 {
			writeError(w, model.ErrInvalidPagination)
			return
		}
		limit, err = strconv.Atoi(value[0])
	}
	if err != nil {
		writeError(w, model.ErrInvalidPagination)
		return
	}
	if value, ok := r.URL.Query()["offset"]; ok {
		if len(value) != 1 {
			writeError(w, model.ErrInvalidPagination)
			return
		}
		offset, err = strconv.Atoi(value[0])
	}
	if err != nil || limit < 1 || limit > model.MaxPageSize || offset < 0 {
		writeError(w, model.ErrInvalidPagination)
		return
	}
	list, err := h.agents.ListByUserID(r.Context(), owner(r), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
