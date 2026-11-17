package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Emulisy/Supa-Agent/internal/service"
)

func (h *handler) runEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = "0-0"
	}
	if err := service.ValidateCursor(cursor); err != nil {
		writeError(w, err)
		return
	}
	events, done, err := h.chat.Events(r.Context(), owner(r), r.PathValue("id"), cursor, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	flush := func(text string) error {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := io.WriteString(w, text); err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := flush(": connected\n\n"); err != nil {
		return
	}
	for {
		for _, event := range events {
			data, err := json.Marshal(event)
			if err != nil {
				return
			}
			if err := flush(fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Type, data)); err != nil {
				return
			}
			cursor = event.ID
			if event.Terminal() {
				return
			}
		}
		if done {
			return
		}
		events, done, err = h.chat.Events(r.Context(), owner(r), r.PathValue("id"), cursor, 15*time.Second)
		if err != nil {
			return
		}
		if len(events) == 0 && !done {
			if err := flush(": heartbeat\n\n"); err != nil {
				return
			}
		}
	}
}
