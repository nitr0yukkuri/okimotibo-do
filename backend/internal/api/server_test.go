package api

import (
	"bytes"
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
	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/store"
	"github.com/coder/websocket"
)

func TestManualStatusDoesNotExpire(t *testing.T) {
	now := time.Now().UTC()
	if !statusExpiresAt(now, 15*time.Minute, domain.SourceManual).After(now.Add(24 * time.Hour)) {
		t.Fatal("manual status should not expire")
	}
	if !statusExpiresAt(now, 15*time.Minute, domain.SourceHand).Before(now.Add(16 * time.Minute)) {
		t.Fatal("automatic status should use configured TTL")
	}
}

func TestPairingClientIsReadOnly(t *testing.T) {
	repository := store.NewMemory()
	server := NewServer(config.Config{AllowAnonymous: true, AllowedOrigins: []string{"*"}}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	request, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/pairing", bytes.NewBufferString(`{"roomId":"room-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer user-1")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("pairing status: %d", response.StatusCode)
	}
	var pairing struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pairing); err != nil || pairing.Token == "" {
		t.Fatalf("pairing response: %#v %v", pairing, err)
	}
	statusRequest, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/v1/rooms/room-1/status/user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	statusRequest.Header.Set("Authorization", "Bearer "+pairing.Token)
	statusResponse, err := http.DefaultClient.Do(statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	statusResponse.Body.Close()
	if statusResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("pairing status authorization: %d", statusResponse.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(testServer.URL, "http")+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"client.hello","token":"`+pairing.Token+`","roomId":"room-1","clientId":"phone-1"}`)); err != nil {
		t.Fatal(err)
	}
	_, ready, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(ready), "server.ready") {
		t.Fatalf("ready: %s %v", ready, err)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"recognition.update","sequence":1,"capturedAt":"`+time.Now().UTC().Format(time.RFC3339Nano)+`","status":"busy","source":"manual"}`)); err != nil {
		t.Fatal(err)
	}
	_, rejected, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(rejected), `"code":"read_only"`) {
		t.Fatalf("read-only rejection: %s %v", rejected, err)
	}
}

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

func TestAnonymousPairingCodeIsSingleUse(t *testing.T) {
	repository := store.NewMemory()
	server := NewServer(config.Config{AllowAnonymous: true, AllowedOrigins: []string{"*"}}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	request, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/pairing", bytes.NewBufferString(`{"roomId":"room-1","userId":"user-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("anonymous pairing status: %d", response.StatusCode)
	}
	var issued struct {
		Token string `json:"token"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&issued); err != nil {
		t.Fatal(err)
	}
	if issued.Token == "" || len(issued.Code) != pairingCodeLength {
		t.Fatalf("anonymous pairing response: %#v", issued)
	}

	// A new API server using the same repository must still be able to claim
	// the code; the production repository is backed by Supabase.
	restarted := NewServer(config.Config{AllowAnonymous: true, AllowedOrigins: []string{"*"}}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
	restartedServer := httptest.NewServer(restarted.Handler())
	defer restartedServer.Close()

	claimRequest, err := http.NewRequest(http.MethodPost, restartedServer.URL+"/api/v1/pairing/claim", bytes.NewBufferString(`{"code":"`+issued.Code+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	claimRequest.Header.Set("Content-Type", "application/json")
	claimResponse, err := http.DefaultClient.Do(claimRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer claimResponse.Body.Close()
	if claimResponse.StatusCode != http.StatusCreated {
		t.Fatalf("claim status: %d", claimResponse.StatusCode)
	}
	var claimed struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(claimResponse.Body).Decode(&claimed); err != nil || claimed.Token != issued.Token {
		t.Fatalf("claim response: %#v %v", claimed, err)
	}
	statusRequest, err := http.NewRequest(http.MethodGet, restartedServer.URL+"/api/v1/rooms/room-1/status/user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	statusRequest.Header.Set("Authorization", "Bearer "+claimed.Token)
	statusResponse, err := http.DefaultClient.Do(statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResponse.Body.Close()
	if statusResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("restarted pairing status authorization: %d", statusResponse.StatusCode)
	}

	secondClaim, err := http.NewRequest(http.MethodPost, restartedServer.URL+"/api/v1/pairing/claim", bytes.NewBufferString(`{"code":"`+issued.Code+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	secondClaim.Header.Set("Content-Type", "application/json")
	secondResponse, err := http.DefaultClient.Do(secondClaim)
	if err != nil {
		t.Fatal(err)
	}
	defer secondResponse.Body.Close()
	if secondResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("second claim status: %d", secondResponse.StatusCode)
	}
}
