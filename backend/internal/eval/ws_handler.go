package eval

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

var evalWSUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// StreamEvalRunEvents handles GET /v1/eval-runs/{eval_run_id}/events.
// It upgrades to WebSocket and streams EvalEvent JSON frames until the EvalRun
// reaches a terminal state (eval.run.completed or eval.run.failed) or the client closes.
//
// For already-terminal runs, synthetic events are sent immediately from the DB
// (one eval.test_case.completed per stored result, then the terminal event).
// For live runs, Subscribe is called before the WebSocket upgrade to avoid
// missing events published during the handshake.
func (h *Handler) StreamEvalRunEvents(w http.ResponseWriter, r *http.Request) {
	evalRunID := r.PathValue("eval_run_id")

	run, err := h.store.GetEvalRun(r.Context(), evalRunID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval run not found")
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}

	isTerminal := run.Status == store.EvalRunCompleted || run.Status == store.EvalRunFailed

	if isTerminal {
		// Fetch stored results before upgrade — r.Context() is valid here.
		results, err := h.store.ListTestCaseResults(r.Context(), evalRunID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}

		conn, err := evalWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("eval ws: upgrade failed", "eval_run_id", evalRunID, "error", err)
			return
		}
		defer conn.Close() //nolint:errcheck
		sendEvalTerminalEvents(conn, run, results)
		return
	}

	// Live run: subscribe BEFORE upgrading to avoid missing events published
	// during the WebSocket handshake.
	if h.bus == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "eval event streaming not available")
		return
	}
	events, cleanup := h.bus.Subscribe(evalRunID)
	defer cleanup()

	conn, err := evalWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("eval ws: upgrade failed", "eval_run_id", evalRunID, "error", err)
		return
	}
	defer conn.Close() //nolint:errcheck

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
				return
			}
			if err := writeEvalWSEvent(conn, evt); err != nil {
				return
			}
			if evt.Type == EvalEventRunCompleted || evt.Type == EvalEventRunFailed {
				return
			}
		case <-clientGone:
			return
		}
	}
}

// sendEvalTerminalEvents writes synthetic events for a completed/failed EvalRun:
// one eval.test_case.completed per stored result, then a terminal run event.
func sendEvalTerminalEvents(conn *websocket.Conn, run store.EvalRun, results []store.TestCaseResult) {
	for i := range results {
		evt := EvalEvent{
			EvalRunID:    run.ID,
			Type:         EvalEventTestCaseCompleted,
			Timestamp:    results[i].CreatedAt,
			TestCaseName: results[i].TestCaseName,
			Result:       &results[i],
		}
		if err := writeEvalWSEvent(conn, evt); err != nil {
			return
		}
	}

	evtType := EvalEventRunCompleted
	if run.Status == store.EvalRunFailed {
		evtType = EvalEventRunFailed
	}
	ts := time.Now().UTC()
	if run.FinishedAt != nil {
		ts = *run.FinishedAt
	}
	_ = writeEvalWSEvent(conn, EvalEvent{
		EvalRunID: run.ID,
		Type:      evtType,
		Timestamp: ts,
		Summary: &EvalRunSummary{
			TotalCases:  run.TotalCases,
			PassedCount: run.PassedCount,
			FailedCount: run.FailedCount,
			ErrorCount:  run.ErrorCount,
		},
	})
}

func writeEvalWSEvent(conn *websocket.Conn, evt EvalEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
