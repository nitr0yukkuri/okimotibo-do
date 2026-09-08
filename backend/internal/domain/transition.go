package domain

import (
	"errors"
	"time"
)

// These errors describe business-rule failures. The transport layer can map
// them to protocol-specific error codes without knowing how transitions work.
var (
	ErrStaleTransition = errors.New("state transition is stale")
	ErrManualPriority  = errors.New("manual state has priority")
	ErrLeaseNotActive  = errors.New("manual lease is not active")
	ErrInvalidLease    = errors.New("manual lease is invalid")
)

type TransitionKind string

const (
	TransitionStatusChanged  TransitionKind = "status.changed"
	TransitionLeaseRefreshed TransitionKind = "lease.refreshed"
	TransitionStatusExpired  TransitionKind = "status.expired"
	TransitionStatusCleared  TransitionKind = "status.cleared"
)

// Transition is the domain result of a command. Previous is populated when a
// current state existed, and Current is nil when the state was cleared.
type Transition struct {
	Kind     TransitionKind
	Previous *State
	Current  *State
}

// ApplyRecognition applies a newly recognized state to the aggregate for one
// room/user pair. Ordering and manual-priority rules live here so Memory,
// Supabase, WebSocket, and future IoT adapters share the same policy.
func ApplyRecognition(current *State, next State, now time.Time) (Transition, error) {
	if err := next.Validate(now); err != nil {
		return Transition{}, err
	}

	var previous *State
	if current != nil {
		previousValue := *current
		previous = &previousValue

		if current.ClientID == next.ClientID && next.Sequence <= current.Sequence {
			return Transition{}, ErrStaleTransition
		}
		if current.Source == SourceManual && next.Source != SourceManual && next.Source != SourceHand && current.ExpiresAt.After(now) {
			return Transition{}, ErrManualPriority
		}
		if next.CapturedAt.Before(current.CapturedAt) ||
			(next.CapturedAt.Equal(current.CapturedAt) && next.ReceivedAt.Before(current.ReceivedAt)) {
			return Transition{}, ErrStaleTransition
		}
	}

	nextValue := next
	return Transition{Kind: TransitionStatusChanged, Previous: previous, Current: &nextValue}, nil
}

// RefreshManualLease renews liveness without pretending a new recognition
// happened. CapturedAt is intentionally preserved: a heartbeat proves the
// device is alive, not that a new camera/gesture sample was captured.
func RefreshManualLease(current State, now time.Time, lease time.Duration) (Transition, error) {
	if current.Source != SourceManual || !current.ExpiresAt.After(now) {
		return Transition{}, ErrLeaseNotActive
	}
	if lease <= 0 {
		return Transition{}, ErrInvalidLease
	}

	previousValue := current
	nextValue := current
	nextValue.ReceivedAt = now
	nextValue.ExpiresAt = now.Add(lease)
	return Transition{
		Kind:     TransitionLeaseRefreshed,
		Previous: &previousValue,
		Current:  &nextValue,
	}, nil
}

// Expire returns a clear transition only when the expected snapshot is
// actually expired. The repository still performs a conditional delete; the
// domain check documents the intended state machine and prevents accidental
// clearing of a live state.
func Expire(current State, now time.Time) (Transition, bool) {
	if current.ExpiresAt.After(now) {
		return Transition{}, false
	}
	previousValue := current
	return Transition{Kind: TransitionStatusExpired, Previous: &previousValue}, true
}

// Clear returns the explicit disconnect clear transition. The caller must use
// a conditional repository operation so a delayed disconnect cannot delete a
// newer state.
func Clear(current State) Transition {
	previousValue := current
	return Transition{Kind: TransitionStatusCleared, Previous: &previousValue}
}
