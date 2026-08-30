package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

var ErrNotFound = errors.New("state not found")
var ErrStaleState = errors.New("state is older than the stored state")

type Repository interface {
	Authenticate(ctx context.Context, token string) (string, error)
	AuthorizeRoom(ctx context.Context, userID, roomID string) error
	UpsertState(ctx context.Context, state domain.State) error
	GetState(ctx context.Context, roomID, userID string) (domain.State, error)
	ClearState(ctx context.Context, roomID, userID, clientID string) (bool, error)
}

// ConditionalClearer prevents a delayed disconnect/expiry job from deleting
// a newer state that happens to belong to the same client.
type ConditionalClearer interface {
	ClearStateIfCurrent(ctx context.Context, roomID, userID, clientID string, sequence uint64, receivedAt time.Time) (bool, error)
}

// ConditionalLeaseRefresher updates a manual lease only when the snapshot
// read by the caller is still the current stored state.
type ConditionalLeaseRefresher interface {
	RefreshManualLeaseIfCurrent(ctx context.Context, current domain.State, capturedAt, receivedAt, expiresAt time.Time) (bool, error)
}

type Memory struct {
	mu           sync.RWMutex
	states       map[string]domain.State
	pairings     map[string]PairingGrant
	pairingCodes map[string]string
}

func NewMemory() *Memory {
	return &Memory{states: make(map[string]domain.State), pairings: make(map[string]PairingGrant), pairingCodes: make(map[string]string)}
}

func (m *Memory) Authenticate(_ context.Context, token string) (string, error) {
	if token == "" {
		return "", errors.New("token is required")
	}
	return token, nil
}

func (m *Memory) AuthorizeRoom(_ context.Context, _, _ string) error { return nil }

func (m *Memory) UpsertState(_ context.Context, state domain.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := state.RoomID + "\x00" + state.UserID
	current, exists := m.states[key]
	if exists && current.Source == domain.SourceManual && state.Source != domain.SourceManual && state.Source != domain.SourceHand && current.ExpiresAt.After(state.ReceivedAt) {
		return ErrStaleState
	}
	if exists && (state.CapturedAt.Before(current.CapturedAt) ||
		(state.CapturedAt.Equal(current.CapturedAt) && state.ReceivedAt.Before(current.ReceivedAt))) {
		return ErrStaleState
	}
	m.states[key] = state
	return nil
}

func (m *Memory) GetState(_ context.Context, roomID, userID string) (domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.states[roomID+"\x00"+userID]
	if !ok || !state.ExpiresAt.After(time.Now().UTC()) {
		if ok {
			delete(m.states, roomID+"\x00"+userID)
		}
		return domain.State{}, ErrNotFound
	}
	return state, nil
}

func (m *Memory) ClearState(_ context.Context, roomID, userID, clientID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := roomID + "\x00" + userID
	state, ok := m.states[key]
	if !ok || state.ClientID != clientID {
		return false, nil
	}
	delete(m.states, key)
	return true, nil
}

func (m *Memory) ClearStateIfCurrent(_ context.Context, roomID, userID, clientID string, sequence uint64, receivedAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := roomID + "\x00" + userID
	state, ok := m.states[key]
	if !ok || state.ClientID != clientID || state.Sequence != sequence || !state.ReceivedAt.Equal(receivedAt) {
		return false, nil
	}
	delete(m.states, key)
	return true, nil
}

func (m *Memory) RefreshManualLeaseIfCurrent(_ context.Context, current domain.State, capturedAt, receivedAt, expiresAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := current.RoomID + "\x00" + current.UserID
	state, ok := m.states[key]
	if !ok || state.ClientID != current.ClientID || state.Sequence != current.Sequence ||
		!state.ReceivedAt.Equal(current.ReceivedAt) || state.Source != domain.SourceManual || !state.ExpiresAt.After(receivedAt) {
		return false, nil
	}
	state.CapturedAt = capturedAt
	state.ReceivedAt = receivedAt
	state.ExpiresAt = expiresAt
	m.states[key] = state
	return true, nil
}
