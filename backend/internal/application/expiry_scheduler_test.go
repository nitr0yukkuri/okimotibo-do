package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

func TestExpirySchedulerReplacesTimerPerAggregate(t *testing.T) {
	now := time.Now().UTC()
	repository := &fakeRepository{}
	state := domain.State{
		RoomID: "room-1", UserID: "user-1", ClientID: "client-1", Sequence: 1,
		CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(70 * time.Millisecond),
		Status: domain.StatusBusy, Source: domain.SourceManual,
	}
	repository.state, repository.found = state, true
	service := NewStatusService(repository, func(err error) bool { return errors.Is(err, errFakeNotFound) })
	expired := make(chan domain.Transition, 1)
	scheduler := NewExpiryScheduler(context.Background(), service, func(transition domain.Transition) { expired <- transition }, nil)
	defer scheduler.Close()

	scheduler.Schedule(state)
	state.ExpiresAt = now.Add(180 * time.Millisecond)
	scheduler.Schedule(state)

	select {
	case transition := <-expired:
		if transition.Kind != domain.TransitionStatusExpired || repository.cleared != 1 {
			t.Fatalf("unexpected expiry: %#v, clears=%d", transition, repository.cleared)
		}
	case <-time.After(time.Second):
		t.Fatal("expiry callback did not run")
	}

	select {
	case <-expired:
		t.Fatal("replaced timer fired more than once")
	case <-time.After(120 * time.Millisecond):
	}
}

func TestExpirySchedulerCloseStopsPendingTimer(t *testing.T) {
	now := time.Now().UTC()
	repository := &fakeRepository{state: domain.State{
		RoomID: "room-1", UserID: "user-1", ClientID: "client-1", Sequence: 1,
		CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(80 * time.Millisecond),
		Status: domain.StatusBusy, Source: domain.SourceManual,
	}, found: true}
	service := NewStatusService(repository, func(err error) bool { return errors.Is(err, errFakeNotFound) })
	expired := make(chan domain.Transition, 1)
	scheduler := NewExpiryScheduler(context.Background(), service, func(transition domain.Transition) { expired <- transition }, nil)
	scheduler.Schedule(repository.state)
	scheduler.Close()

	select {
	case <-expired:
		t.Fatal("closed scheduler emitted expiry")
	case <-time.After(150 * time.Millisecond):
	}
}
