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
// For already-terminal runs, a synthetic event is sent immediately from the
// DB record with no subscription overhead. For live runs, Subscribe is called
// before the WebSocket upgrade to avoid missing events during the handshake.
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

	// Fast path: run already finished — upgrade, send synthetic event, close.
	// No subscription needed; terminal state is authoritative in the DB.
	if isTerminalStatus(run.Status) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("ws: upgrade failed", "run_id", runID, "error", err)
			return
		}
		defer conn.Close()
		sendTerminalEvent(conn, run)
		return
	}

	// Live run: subscribe BEFORE upgrading to avoid missing events published
	// during the WebSocket handshake.
	events, cleanup := h.bus.Subscribe(runID)
	defer cleanup()

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "run_id", runID, "error", err)
		return
	}
	defer conn.Close()

	// Read pump: gorilla/websocket requires continuous reads to process control
	// frames (ping/pong/close). Closing clientGone signals a disconnect.
	// r.Context() is not used here — the net/http context is cancelled by the
	// server after hijack, so clientGone is the authoritative disconnect signal.
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
	ts := time.Now().UTC()
	if run.FinishedAt != nil {
		ts = *run.FinishedAt
	}
	_ = writeWSEvent(conn, engine.NodeEvent{
		RunID:     run.ID,
		Type:      evtType,
		Error:     errMsg,
		Timestamp: ts,
	})
}

func writeWSEvent(conn *websocket.Conn, evt engine.NodeEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
