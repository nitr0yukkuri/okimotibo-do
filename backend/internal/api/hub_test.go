package api

import "testing"

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
