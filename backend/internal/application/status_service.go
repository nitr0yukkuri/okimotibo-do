package application

import (
	"context"
	"errors"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

// StateRepository is the application port used by the status use case. The
// concrete Memory and Supabase stores remain infrastructure details.
type StateRepository interface {
	UpsertState(ctx context.Context, state domain.State) error
	GetState(ctx context.Context, roomID, userID string) (domain.State, error)
}

type ConditionalClearer interface {
	ClearStateIfCurrent(ctx context.Context, roomID, userID, clientID string, sequence uint64, receivedAt time.Time) (bool, error)
}

type ConditionalLeaseRefresher interface {
	RefreshManualLeaseIfCurrent(ctx context.Context, current domain.State, capturedAt, receivedAt, expiresAt time.Time) (bool, error)
}

var (
	ErrStateNotFound           = errors.New("state not found")
	ErrConditionalClearMissing = errors.New("conditional state clear is unsupported")
	ErrLeaseRefreshUnsupported = errors.New("manual lease refresh is unsupported")
	ErrLeaseOwnerMismatch      = errors.New("manual lease owner mismatch")
)

// StatusService is the application layer for status commands. It coordinates
// loading, domain transition, and persistence, while the API layer decides how
// to serialize the returned transition to WebSocket/HTTP clients.
type StatusService struct {
	repository StateRepository
	isNotFound func(error) bool
}

func NewStatusService(repository StateRepository, isNotFound func(error) bool) *StatusService {
	if isNotFound == nil {
		isNotFound = func(error) bool { return false }
	}
	return &StatusService{repository: repository, isNotFound: isNotFound}
}

func (s *StatusService) GetCurrent(ctx context.Context, roomID, userID string) (domain.State, error) {
	state, found, err := s.load(ctx, roomID, userID)
	if err != nil {
		return domain.State{}, err
	}
	if !found {
		return domain.State{}, ErrStateNotFound
	}
	return state, nil
}

func (s *StatusService) ApplyRecognition(ctx context.Context, next domain.State, now time.Time) (domain.Transition, error) {
	current, found, err := s.load(ctx, next.RoomID, next.UserID)
	if err != nil {
		return domain.Transition{}, err
	}
	var currentPtr *domain.State
	if found {
		currentPtr = &current
	}
	transition, err := domain.ApplyRecognition(currentPtr, next, now)
	if err != nil {
		return domain.Transition{}, err
	}
	if err := s.repository.UpsertState(ctx, *transition.Current); err != nil {
		return domain.Transition{}, err
	}
	return transition, nil
}

func (s *StatusService) RefreshManualLease(ctx context.Context, roomID, userID, clientID string, now time.Time, lease time.Duration) (domain.Transition, error) {
	current, found, err := s.load(ctx, roomID, userID)
	if err != nil {
		return domain.Transition{}, err
	}
	if !found {
		return domain.Transition{}, ErrStateNotFound
	}
	if current.ClientID != clientID {
		return domain.Transition{}, ErrLeaseOwnerMismatch
	}
	transition, err := domain.RefreshManualLease(current, now, lease)
	if err != nil {
		return domain.Transition{}, err
	}
	refresher, ok := s.repository.(ConditionalLeaseRefresher)
	if !ok {
		return domain.Transition{}, ErrLeaseRefreshUnsupported
	}
	refreshed, err := refresher.RefreshManualLeaseIfCurrent(ctx, current, transition.Current.CapturedAt, transition.Current.ReceivedAt, transition.Current.ExpiresAt)
	if err != nil {
		return domain.Transition{}, err
	}
	if !refreshed {
		return domain.Transition{}, domain.ErrStaleTransition
	}
	return transition, nil
}

func (s *StatusService) Expire(ctx context.Context, expected domain.State, now time.Time) (domain.Transition, bool, error) {
	transition, shouldClear := domain.Expire(expected, now)
	if !shouldClear {
		return domain.Transition{}, false, nil
	}
	cleared, err := s.clearExpected(ctx, expected)
	if err != nil || !cleared {
		return domain.Transition{}, cleared, err
	}
	return transition, true, nil
}

func (s *StatusService) Clear(ctx context.Context, expected domain.State) (domain.Transition, bool, error) {
	cleared, err := s.clearExpected(ctx, expected)
	if err != nil || !cleared {
		return domain.Transition{}, cleared, err
	}
	return domain.Clear(expected), true, nil
}

func (s *StatusService) load(ctx context.Context, roomID, userID string) (domain.State, bool, error) {
	state, err := s.repository.GetState(ctx, roomID, userID)
	if s.isNotFound(err) {
		return domain.State{}, false, nil
	}
	if err != nil {
		return domain.State{}, false, err
	}
	return state, true, nil
}

func (s *StatusService) clearExpected(ctx context.Context, expected domain.State) (bool, error) {
	if clearer, ok := s.repository.(ConditionalClearer); ok {
		return clearer.ClearStateIfCurrent(ctx, expected.RoomID, expected.UserID, expected.ClientID, expected.Sequence, expected.ReceivedAt)
	}
	return false, ErrConditionalClearMissing
}
