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

func TestMemoryManualStateBlocksFaceButAllowsHand(t *testing.T) {
	repository := NewMemory()
	now := time.Now().UTC()
	manual := domain.State{
		RoomID: "room-a", UserID: "user-a", ClientID: "manual", Sequence: 1,
		CapturedAt: now, ReceivedAt: now, ExpiresAt: time.Date(9999, time.December, 31, 23, 59, 59, 999999999, time.UTC),
		Status: domain.StatusBusy, Source: domain.SourceManual,
	}
	if err := repository.UpsertState(context.Background(), manual); err != nil {
		t.Fatal(err)
	}

	face := manual
	face.ClientID = "face"
	face.Sequence = 2
	face.CapturedAt = now.Add(time.Second)
	face.ReceivedAt = now.Add(time.Second)
	face.Status = domain.StatusAvailable
	face.Source = domain.SourceFace
	face.Face = &domain.Face{Expression: "smile", Confidence: .9}
	if err := repository.UpsertState(context.Background(), face); !errors.Is(err, ErrStaleState) {
		t.Fatalf("expected manual state to block face state, got %v", err)
	}

	hand := manual
	hand.ClientID = "camera"
	hand.Sequence = 3
	hand.CapturedAt = now.Add(2 * time.Second)
	hand.ReceivedAt = now.Add(2 * time.Second)
	hand.ExpiresAt = now.Add(15 * time.Minute)
	hand.Status = domain.StatusAvailable
	hand.Source = domain.SourceHand
	hand.Hand = &domain.Hand{Gesture: "thumb_up", Confidence: .9}
	if err := repository.UpsertState(context.Background(), hand); err != nil {
		t.Fatalf("expected hand state to override manual state, got %v", err)
	}
}
