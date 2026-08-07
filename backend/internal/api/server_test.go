package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/config"
	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/store"
	"github.com/coder/websocket"
)

func TestWebSocketRecognitionRoundTrip(t *testing.T) {
	repository := store.NewMemory()
	server := NewServer(config.Config{AllowAnonymous: true, AllowedOrigins: []string{"*"}}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(testServer.URL, "http")+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	hello := `{"type":"client.hello","token":"","roomId":"room-1","clientId":"client-1","userId":"user-1"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(hello)); err != nil {
		t.Fatal(err)
	}
	_, ready, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(ready), "server.ready") {
		t.Fatalf("ready: %s %v", ready, err)
	}

	update := `{"type":"recognition.update","sequence":1,"capturedAt":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","status":"available","source":"hand","hand":{"gesture":"thumb_up","confidence":0.9}}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(update)); err != nil {
		t.Fatal(err)
	}
	_, changed, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(changed), "status.changed") {
		t.Fatalf("changed: %s %v", changed, err)
	}

	response, err := http.Get(testServer.URL + "/api/v1/rooms/room-1/status/user-1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var state map[string]any
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state["status"] != "available" {
		t.Fatalf("unexpected state: %#v", state)
	}
	if _, ok := state["expiresAt"].(string); !ok {
		t.Fatalf("expiresAt missing from state: %#v", state)
	}
}

func TestWebSocketManualRoundTrip(t *testing.T) {
	repository := store.NewMemory()
	server := NewServer(config.Config{AllowAnonymous: true, AllowedOrigins: []string{"*"}}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(testServer.URL, "http")+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	hello := `{"type":"client.hello","token":"","roomId":"room-1","clientId":"client-1","userId":"user-1"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(hello)); err != nil {
		t.Fatal(err)
	}
	_, ready, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(ready), "server.ready") {
		t.Fatalf("ready: %s %v", ready, err)
	}

	update := `{"type":"recognition.update","sequence":1,"capturedAt":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","status":"busy","source":"manual"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(update)); err != nil {
		t.Fatal(err)
	}
	_, changed, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(changed), "status.changed") {
		t.Fatalf("changed: %s %v", changed, err)
	}

	response, err := http.Get(testServer.URL + "/api/v1/rooms/room-1/status/user-1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var state map[string]any
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state["status"] != "busy" {
		t.Fatalf("unexpected state: %#v", state)
	}
	if state["source"] != "manual" {
		t.Fatalf("unexpected source: %#v", state)
	}
}
