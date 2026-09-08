package domain

import (
	"errors"
	"testing"
	"time"
)

func validState(now time.Time) State {
	return State{
		RoomID: "room-1", UserID: "user-1", ClientID: "client-1", Sequence: 1,
		CapturedAt: now, ReceivedAt: now, ExpiresAt: now.Add(time.Minute),
		Status: StatusBusy, Source: SourceManual,
	}
}

func TestApplyRecognitionOwnsOrderingAndManualPriority(t *testing.T) {
	now := time.Now().UTC()
	current := validState(now)

	next := current
	next.Sequence = 2
	next.ReceivedAt = now.Add(time.Second)
	transition, err := ApplyRecognition(&current, next, now)
	if err != nil || transition.Current == nil || transition.Current.Sequence != 2 {
		t.Fatalf("expected accepted transition, got %#v, %v", transition, err)
	}

	stale := next
	stale.Sequence = 1
	if _, err := ApplyRecognition(&current, stale, now); !errors.Is(err, ErrStaleTransition) {
		t.Fatalf("expected stale sequence error, got %v", err)
	}

	face := next
	face.Source = SourceFace
	face.Face = &Face{Expression: "frown", Confidence: 0.9}
	face.Hand = nil
	face.Status = StatusBusy
	if _, err := ApplyRecognition(&current, face, now); !errors.Is(err, ErrManualPriority) {
		t.Fatalf("expected manual priority error, got %v", err)
	}
}

func TestRefreshManualLeasePreservesCaptureTime(t *testing.T) {
	now := time.Now().UTC()
	capturedAt := now.Add(-10 * time.Second)
	current := validState(now)
	current.CapturedAt = capturedAt
	transition, err := RefreshManualLease(current, now, 30*time.Second)
	if err != nil || transition.Kind != TransitionLeaseRefreshed || transition.Current == nil {
		t.Fatalf("expected lease refresh, got %#v, %v", transition, err)
	}
	if !transition.Current.CapturedAt.Equal(capturedAt) || !transition.Current.ReceivedAt.Equal(now) {
		t.Fatalf("heartbeat changed recognition timestamp: %#v", transition.Current)
	}
}

func TestExpireOnlyEmitsForExpiredState(t *testing.T) {
	now := time.Now().UTC()
	current := validState(now)
	if _, ok := Expire(current, now); ok {
		t.Fatal("live state must not expire")
	}
	current.ExpiresAt = now.Add(-time.Second)
	transition, ok := Expire(current, now)
	if !ok || transition.Kind != TransitionStatusExpired || transition.Previous == nil || transition.Current != nil {
		t.Fatalf("unexpected expiration transition: %#v, %t", transition, ok)
	}
}
