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
}

type Memory struct {
	mu     sync.RWMutex
	states map[string]domain.State
}

func NewMemory() *Memory {
	return &Memory{states: make(map[string]domain.State)}
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
