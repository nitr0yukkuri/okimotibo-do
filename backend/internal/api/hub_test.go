package api

import (
	"testing"
)

func TestHubHasPublisherIgnoresReadOnlyClients(t *testing.T) {
	h := newHub()
	writable := &client{roomID: "room-1", userID: "user-1", clientID: "pc-1"}
	readOnly := &client{roomID: "room-1", userID: "user-1", clientID: "phone-1", readOnly: true}
	h.add(writable)
	h.add(readOnly)

	if !h.hasPublisher("room-1", "user-1") {
		t.Fatal("expected writable client to count as a publisher")
	}
	h.remove(writable)
	if h.hasPublisher("room-1", "user-1") {
		t.Fatal("read-only client must not keep a publisher alive")
	}
}

func TestHubBootstrapSendsReadyBeforeEvents(t *testing.T) {
	hub := newHub()
	client := &client{
		roomID: "room-1", userID: "user-1", clientID: "client-1",
		send: make(chan []byte, 16), cancel: func() {},
	}
	hub.add(client)
	hub.broadcast("room-1", []byte("status.changed"))
	if len(client.send) != 0 || len(client.pending) != 1 {
		t.Fatalf("event escaped bootstrap gate: send=%d pending=%d", len(client.send), len(client.pending))
	}
	if !hub.finishBootstrap(client, []byte("server.ready")) {
		t.Fatal("bootstrap should finish")
	}
	if got := string(<-client.send); got != "server.ready" {
		t.Fatalf("first message = %q", got)
	}
	if got := string(<-client.send); got != "status.changed" {
		t.Fatalf("second message = %q", got)
	}
}

func TestHubBootstrapBoundsPendingEvents(t *testing.T) {
	hub := newHub()
	canceled := false
	client := &client{
		roomID: "room-1", userID: "user-1", clientID: "client-1",
		send: make(chan []byte, 16), cancel: func() { canceled = true },
	}
	hub.add(client)
	for i := 0; i < cap(client.send); i++ {
		hub.broadcast("room-1", []byte("event"))
	}
	if !canceled {
		t.Fatal("bootstrap client should be canceled when its pending queue is full")
	}
}
