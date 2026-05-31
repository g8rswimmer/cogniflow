package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- helpers ----------------------------------------------------------------

func newWSTestServer(t *testing.T, h *wsHandler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /runs/{run_id}/events", h.streamEvents)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func setupWSHandler(t *testing.T) (*wsHandler, *mockStore, *engine.EventBus) {
	t.Helper()
	ms := newMockStore()
	bus := engine.NewEventBus()
	return &wsHandler{store: ms, bus: bus}, ms, bus
}

// wsDialRaw dials the WebSocket endpoint without failing the test on error —
// the caller inspects the HTTP response for negative cases.
func wsDialRaw(srv *httptest.Server, path string) (*websocket.Conn, *http.Response, error) {
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + path
	return websocket.DefaultDialer.Dial(u, nil) //nolint:wrapcheck
}

// wsDial dials and fails the test if the dial fails.
func wsDial(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	conn, _, err := wsDialRaw(srv, path)
	if err != nil {
		t.Fatalf("ws dial %s: %v", path, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readEvent reads one JSON frame and unmarshals it into a NodeEvent.
func readEvent(t *testing.T, conn *websocket.Conn) engine.NodeEvent {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var evt engine.NodeEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return evt
}

// expectClose reads one more message from conn and expects a server-side close.
// It fails the test if the read succeeds (server didn't close) or if the only
// error is a deadline timeout (server stalled without closing).
func expectClose(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Error("expected connection to be closed by server, but read succeeded")
		return
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Errorf("timed out waiting for server to close the connection: %v", err)
	}
}

// ---- tests ------------------------------------------------------------------

func TestWSHandler_RunNotFound(t *testing.T) {
	h, _, _ := setupWSHandler(t)
	srv := newWSTestServer(t, h)

	_, resp, err := wsDialRaw(srv, "/runs/nonexistent/events")
	if err == nil {
		t.Fatal("expected dial error for nonexistent run")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 response, got %v", resp)
	}
}

func TestWSHandler_StoreError(t *testing.T) {
	h, ms, _ := setupWSHandler(t)
	ms.getRunErr = errInternal
	srv := newWSTestServer(t, h)

	_, resp, err := wsDialRaw(srv, "/runs/any/events")
	if err == nil {
		t.Fatal("expected dial error on store failure")
	}
	if resp == nil || resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 response, got %v", resp)
	}
}

func TestWSHandler_AlreadySucceeded(t *testing.T) {
	h, ms, _ := setupWSHandler(t)
	ms.runs["run-done"] = store.Run{
		ID:     "run-done",
		Status: store.RunStatusSucceeded,
	}
	srv := newWSTestServer(t, h)

	conn := wsDial(t, srv, "/runs/run-done/events")

	evt := readEvent(t, conn)
	if evt.Type != engine.EventRunSucceeded {
		t.Errorf("expected run.succeeded, got %s", evt.Type)
	}
	if evt.RunID != "run-done" {
		t.Errorf("expected run_id=run-done, got %s", evt.RunID)
	}

	expectClose(t, conn)
}

func TestWSHandler_AlreadyFailed(t *testing.T) {
	h, ms, _ := setupWSHandler(t)
	ms.runs["run-fail"] = store.Run{
		ID:          "run-fail",
		Status:      store.RunStatusFailed,
		ErrorDetail: map[string]any{"error": "node timed out"},
	}
	srv := newWSTestServer(t, h)

	conn := wsDial(t, srv, "/runs/run-fail/events")

	evt := readEvent(t, conn)
	if evt.Type != engine.EventRunFailed {
		t.Errorf("expected run.failed, got %s", evt.Type)
	}
	if evt.Error != "node timed out" {
		t.Errorf("expected error message, got %q", evt.Error)
	}

	expectClose(t, conn)
}

func TestWSHandler_LiveEvents(t *testing.T) {
	h, ms, bus := setupWSHandler(t)
	ms.runs["run-live"] = store.Run{ID: "run-live", Status: store.RunStatusRunning}
	srv := newWSTestServer(t, h)

	// Dial first — Subscribe is called before the upgrade, so by the time Dial
	// returns the handler has already registered its subscriber.
	conn := wsDial(t, srv, "/runs/run-live/events")

	// Publish a node lifecycle and then the terminal run event.
	now := time.Now().UTC()
	bus.Publish(engine.NodeEvent{RunID: "run-live", NodeID: "n1", Type: engine.EventNodeRunning, Timestamp: now})
	bus.Publish(engine.NodeEvent{RunID: "run-live", NodeID: "n1", Type: engine.EventNodeSucceeded, Timestamp: now, Output: map[string]any{"ok": true}})
	bus.Publish(engine.NodeEvent{RunID: "run-live", Type: engine.EventRunSucceeded, Timestamp: now})

	e1 := readEvent(t, conn)
	if e1.Type != engine.EventNodeRunning {
		t.Errorf("event 1: expected node.running, got %s", e1.Type)
	}

	e2 := readEvent(t, conn)
	if e2.Type != engine.EventNodeSucceeded {
		t.Errorf("event 2: expected node.succeeded, got %s", e2.Type)
	}

	e3 := readEvent(t, conn)
	if e3.Type != engine.EventRunSucceeded {
		t.Errorf("event 3: expected run.succeeded, got %s", e3.Type)
	}

	// Handler returns after terminal event; connection closes.
	expectClose(t, conn)
}

func TestWSHandler_LiveEvents_RunFailed(t *testing.T) {
	h, ms, bus := setupWSHandler(t)
	ms.runs["run-err"] = store.Run{ID: "run-err", Status: store.RunStatusRunning}
	srv := newWSTestServer(t, h)

	conn := wsDial(t, srv, "/runs/run-err/events")

	now := time.Now().UTC()
	bus.Publish(engine.NodeEvent{RunID: "run-err", NodeID: "n1", Type: engine.EventNodeFailed, Error: "timeout", Timestamp: now})
	bus.Publish(engine.NodeEvent{RunID: "run-err", Type: engine.EventRunFailed, Error: "timeout", Timestamp: now})

	e1 := readEvent(t, conn)
	if e1.Type != engine.EventNodeFailed {
		t.Errorf("expected node.failed, got %s", e1.Type)
	}

	e2 := readEvent(t, conn)
	if e2.Type != engine.EventRunFailed {
		t.Errorf("expected run.failed, got %s", e2.Type)
	}

	expectClose(t, conn)
}

func TestWSHandler_ClientDisconnect(t *testing.T) {
	h, ms, _ := setupWSHandler(t)
	ms.runs["run-dc"] = store.Run{ID: "run-dc", Status: store.RunStatusRunning}
	srv := newWSTestServer(t, h)

	conn, _, err := wsDialRaw(srv, "/runs/run-dc/events")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Close the client connection immediately; the handler should detect it via
	// clientGone and return cleanly without blocking.
	conn.Close()

	// Allow the handler goroutine to detect the close and unwind.
	time.Sleep(50 * time.Millisecond)
}
