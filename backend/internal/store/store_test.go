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

func TestMemoryClearStateOnlyRemovesOwnedClient(t *testing.T) {
	repository := NewMemory()
	now := time.Now().UTC()
	state := domain.State{
		RoomID: "room-a", UserID: "user-a", ClientID: "client-a", CapturedAt: now,
		ReceivedAt: now, ExpiresAt: now.Add(time.Minute), Status: domain.StatusBusy,
	}
	if err := repository.UpsertState(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if cleared, err := repository.ClearState(context.Background(), "room-a", "user-a", "client-b"); err != nil || cleared {
		t.Fatalf("a different client must not clear the state: cleared=%v err=%v", cleared, err)
	}
	if _, err := repository.GetState(context.Background(), "room-a", "user-a"); err != nil {
		t.Fatalf("state was cleared by a different client: %v", err)
	}
	if cleared, err := repository.ClearState(context.Background(), "room-a", "user-a", "client-a"); err != nil || !cleared {
		t.Fatalf("the publishing client should clear the state: cleared=%v err=%v", cleared, err)
	}
	if _, err := repository.GetState(context.Background(), "room-a", "user-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cleared state to be missing, got %v", err)
	}
}

func TestMemoryConditionalClearDoesNotRemoveNewerStateFromSameClient(t *testing.T) {
	repository := NewMemory()
	firstReceived := time.Now().UTC()
	first := domain.State{
		RoomID: "room-a", UserID: "user-a", ClientID: "client-a", Sequence: 1,
		CapturedAt: firstReceived, ReceivedAt: firstReceived, ExpiresAt: firstReceived.Add(time.Minute), Status: domain.StatusBusy,
	}
	if err := repository.UpsertState(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	newer := first
	newer.Sequence = 2
	newer.CapturedAt = firstReceived.Add(time.Second)
	newer.ReceivedAt = firstReceived.Add(time.Second)
	if err := repository.UpsertState(context.Background(), newer); err != nil {
		t.Fatal(err)
	}
	cleared, err := repository.ClearStateIfCurrent(context.Background(), first.RoomID, first.UserID, first.ClientID, first.Sequence, first.ReceivedAt)
	if err != nil {
		t.Fatal(err)
	}
	if cleared {
		t.Fatal("a delayed clear must not remove a newer state from the same client")
	}
	current, err := repository.GetState(context.Background(), newer.RoomID, newer.UserID)
	if err != nil || current.Sequence != newer.Sequence {
		t.Fatalf("newer state was lost: %#v %v", current, err)
	}
}

func TestMemoryConditionalLeaseRefreshDoesNotOverwriteNewerState(t *testing.T) {
	repository := NewMemory()
	firstReceived := time.Now().UTC()
	first := domain.State{
		RoomID: "room-a", UserID: "user-a", ClientID: "client-a", Sequence: 1,
		CapturedAt: firstReceived, ReceivedAt: firstReceived, ExpiresAt: firstReceived.Add(time.Minute), Status: domain.StatusBusy, Source: domain.SourceManual,
	}
	if err := repository.UpsertState(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	newer := first
	newer.Sequence = 2
	newer.CapturedAt = firstReceived.Add(time.Second)
	newer.ReceivedAt = firstReceived.Add(time.Second)
	newer.Status = domain.StatusAvailable
	if err := repository.UpsertState(context.Background(), newer); err != nil {
		t.Fatal(err)
	}

	refreshed, err := repository.RefreshManualLeaseIfCurrent(context.Background(), first, time.Now().UTC(), time.Now().UTC().Add(time.Minute), time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if refreshed {
		t.Fatal("a stale heartbeat must not refresh a newer state")
	}
	current, err := repository.GetState(context.Background(), newer.RoomID, newer.UserID)
	if err != nil || current.Sequence != newer.Sequence || current.Status != newer.Status {
		t.Fatalf("newer state was overwritten: %#v %v", current, err)
	}
}

func TestMemoryConditionalLeaseRefreshUpdatesCurrentManualState(t *testing.T) {
	repository := NewMemory()
	firstReceived := time.Now().UTC()
	first := domain.State{
		RoomID: "room-a", UserID: "user-a", ClientID: "client-a", Sequence: 1,
		CapturedAt: firstReceived, ReceivedAt: firstReceived, ExpiresAt: firstReceived.Add(time.Second), Status: domain.StatusBusy, Source: domain.SourceManual,
	}
	if err := repository.UpsertState(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	capturedAt := firstReceived.Add(500 * time.Millisecond)
	receivedAt := firstReceived.Add(500 * time.Millisecond)
	expiresAt := receivedAt.Add(time.Minute)
	refreshed, err := repository.RefreshManualLeaseIfCurrent(context.Background(), first, capturedAt, receivedAt, expiresAt)
	if err != nil || !refreshed {
		t.Fatalf("expected current manual lease to refresh: refreshed=%v err=%v", refreshed, err)
	}
	current, err := repository.GetState(context.Background(), first.RoomID, first.UserID)
	if err != nil || !current.ReceivedAt.Equal(receivedAt) || !current.ExpiresAt.Equal(expiresAt) || current.Status != first.Status {
		t.Fatalf("manual lease refresh was not applied: %#v %v", current, err)
	}
}
