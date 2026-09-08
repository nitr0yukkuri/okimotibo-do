package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

type fakeRepository struct {
	state     domain.State
	found     bool
	upserted  int
	refreshed int
	cleared   int
}

func (f *fakeRepository) UpsertState(_ context.Context, state domain.State) error {
	f.state, f.found, f.upserted = state, true, f.upserted+1
	return nil
}
func (f *fakeRepository) GetState(_ context.Context, _, _ string) (domain.State, error) {
	if !f.found {
		return domain.State{}, errFakeNotFound
	}
	return f.state, nil
}
func (f *fakeRepository) ClearState(_ context.Context, _, _, _ string) (bool, error) {
	f.found, f.cleared = false, f.cleared+1
	return true, nil
}
func (f *fakeRepository) ClearStateIfCurrent(_ context.Context, _, _, _ string, sequence uint64, receivedAt time.Time) (bool, error) {
	if !f.found || f.state.Sequence != sequence || !f.state.ReceivedAt.Equal(receivedAt) {
		return false, nil
	}
	f.found, f.cleared = false, f.cleared+1
	return true, nil
}
func (f *fakeRepository) RefreshManualLeaseIfCurrent(_ context.Context, current domain.State, _, receivedAt, expiresAt time.Time) (bool, error) {
	if !f.found || f.state.ClientID != current.ClientID || f.state.Sequence != current.Sequence || !f.state.ReceivedAt.Equal(current.ReceivedAt) {
		return false, nil
	}
	f.state.ReceivedAt, f.state.ExpiresAt, f.refreshed = receivedAt, expiresAt, f.refreshed+1
	return true, nil
}

var errFakeNotFound = errors.New("fake not found")

func TestStatusServiceKeepsTransitionRulesOutOfTransport(t *testing.T) {
	now := time.Now().UTC()
	repository := &fakeRepository{}
	service := NewStatusService(repository, func(err error) bool { return errors.Is(err, errFakeNotFound) })
	state := domain.State{
		RoomID: "room-1", UserID: "user-1", ClientID: "client-1", Sequence: 1,
		CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(time.Minute),
		Status: domain.StatusBusy, Source: domain.SourceManual,
	}
	if _, err := service.ApplyRecognition(context.Background(), state, now); err != nil {
		t.Fatal(err)
	}
	next := state
	next.Sequence = 2
	next.ReceivedAt = now.Add(time.Second)
	if _, err := service.ApplyRecognition(context.Background(), next, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if repository.upserted != 2 {
		t.Fatalf("expected two persisted transitions, got %d", repository.upserted)
	}
}

func TestStatusServiceRefreshesAndExpiresConditionally(t *testing.T) {
	now := time.Now().UTC()
	repository := &fakeRepository{}
	service := NewStatusService(repository, func(err error) bool { return errors.Is(err, errFakeNotFound) })
	state := domain.State{
		RoomID: "room-1", UserID: "user-1", ClientID: "iot-1", Sequence: 1,
		CapturedAt: now.Add(-time.Second), ReceivedAt: now, ExpiresAt: now.Add(time.Second),
		Status: domain.StatusAvailable, Source: domain.SourceManual,
	}
	repository.state, repository.found = state, true
	transition, err := service.RefreshManualLease(context.Background(), "room-1", "user-1", "iot-1", now, 30*time.Second)
	if err != nil || transition.Kind != domain.TransitionLeaseRefreshed || repository.refreshed != 1 {
		t.Fatalf("expected lease refresh, got %#v, %v", transition, err)
	}

	repository.state.ExpiresAt = now.Add(-time.Second)
	transition, cleared, err := service.Expire(context.Background(), repository.state, now)
	if err != nil || !cleared || transition.Kind != domain.TransitionStatusExpired || repository.cleared != 1 {
		t.Fatalf("expected conditional expiry, got %#v, %t, %v", transition, cleared, err)
	}
}
