package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/eigeninference/d-inference/coordinator/store"
)

func (s *Server) handleAdminAutopilot(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminAuthorized(w, r) {
		return
	}
	if r.Method == http.MethodPost {
		var body struct {
			Paused *bool `json:"paused"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.Paused == nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid_request_error", "paused is required"))
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid_request_error", "exactly one JSON object is required"))
			return
		}
		if !s.registry.SetAutopilotPaused(*body.Paused) {
			writeJSON(w, http.StatusConflict, errorResponse("invalid_request_error", "Autopilot is not configured"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": s.registry.AutopilotSnapshot()})
		return
	}
	records := []store.AutopilotRecord{}
	if ledger, ok := store.As[store.AutopilotStore](s.store); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var err error
		records, err = ledger.AutopilotRecords(ctx, time.Now().Add(-24*time.Hour), 200)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse("server_error", "Autopilot ledger unavailable"))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": s.registry.AutopilotSnapshot(), "events": records})
}
