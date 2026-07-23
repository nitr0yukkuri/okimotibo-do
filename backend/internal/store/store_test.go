package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

func TestMemorySeparatesRoomsAndRejectsOlderState(t *testing.T) {
	repository := NewMemory()
	now := time.Now().UTC()
	newer := domain.State{RoomID: "room-a", UserID: "user-a", CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	older := newer
	older.CapturedAt = now.Add(-time.Second)

	if err := repository.UpsertState(context.Background(), newer); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpsertState(context.Background(), older); !errors.Is(err, ErrStaleState) {
		t.Fatalf("expected stale error, got %v", err)
	}
	if _, err := repository.GetState(context.Background(), "room-b", "user-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("state leaked across rooms: %v", err)
	}
}

func TestMemoryDoesNotReturnExpiredState(t *testing.T) {
	repository := NewMemory()
	now := time.Now().UTC()
	state := domain.State{
		RoomID: "room-a", UserID: "user-a", CapturedAt: now.Add(-16 * time.Minute),
		ReceivedAt: now.Add(-16 * time.Minute), ExpiresAt: now.Add(-time.Minute),
	}
	if err := repository.UpsertState(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetState(context.Background(), "room-a", "user-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expired state to be hidden, got %v", err)
	}
}
