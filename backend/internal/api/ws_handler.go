package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

var wsUpgrader = websocket.Upgrader{
	// Allow all origins; tighten in production behind a reverse proxy.
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type wsHandler struct {
	store store.Store
	bus   *engine.EventBus
}

// streamEvents handles GET /runs/{run_id}/events.
// It upgrades to WebSocket and streams NodeEvent JSON frames until the run
// reaches a terminal state (run.succeeded or run.failed) or the client closes.
//
// Subscribe is called before the upgrade so no events are missed during the
// WebSocket handshake. If the run is already terminal when the client connects,
// a synthetic terminal event is sent immediately from the DB record.
func (h *wsHandler) streamEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")

	// Verify run exists before upgrading — HTTP error responses cannot be sent
	// after the connection is hijacked for WebSocket.
	run, err := h.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}

	// Subscribe before upgrading so no events are missed during the handshake.
	events, cleanup := h.bus.Subscribe(runID)
	defer cleanup()

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "run_id", runID, "error", err)
		return
	}
	defer conn.Close()

	// Run already finished; synthesize the terminal event from the DB record and close.
	if isTerminalStatus(run.Status) {
		sendTerminalEvent(conn, run)
		return
	}

	// Read pump: gorilla/websocket requires continuous reads to process control
	// frames (ping/pong/close). Closing clientGone signals a disconnect.
	clientGone := make(chan struct{})
	go func() {
		defer close(clientGone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				// Channel closed — run is done but we may have missed the terminal event.
				return
			}
			if err := writeWSEvent(conn, evt); err != nil {
				return
			}
			if isTerminalEventType(evt.Type) {
				return
			}
		case <-clientGone:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func isTerminalStatus(s store.RunStatus) bool {
	return s == store.RunStatusSucceeded || s == store.RunStatusFailed
}

func isTerminalEventType(t engine.NodeEventType) bool {
	return t == engine.EventRunSucceeded || t == engine.EventRunFailed
}

// sendTerminalEvent writes a synthetic run.succeeded or run.failed event
// derived from the stored run record.
func sendTerminalEvent(conn *websocket.Conn, run store.Run) {
	evtType := engine.EventRunSucceeded
	errMsg := ""
	if run.Status == store.RunStatusFailed {
		evtType = engine.EventRunFailed
		if run.ErrorDetail != nil {
			if msg, ok := run.ErrorDetail["error"].(string); ok {
				errMsg = msg
			}
		}
	}
	_ = writeWSEvent(conn, engine.NodeEvent{
		RunID:     run.ID,
		Type:      evtType,
		Error:     errMsg,
		Timestamp: time.Now().UTC(),
	})
}

func writeWSEvent(conn *websocket.Conn, evt engine.NodeEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
