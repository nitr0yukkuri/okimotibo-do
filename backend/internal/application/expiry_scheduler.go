package application

import (
	"context"
	"sync"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

type ExpiryErrorHandler func(error, domain.State)

type expiryEntry struct {
	generation uint64
	timer      *time.Timer
}

// ExpiryScheduler keeps one replaceable timer per room/user aggregate. This
// avoids creating a timer for every camera frame or heartbeat while retaining
// conditional deletion in StatusService as the final concurrency guard.
type ExpiryScheduler struct {
	service   *StatusService
	ctx       context.Context
	cancel    context.CancelFunc
	onExpired func(domain.Transition)
	onError   ExpiryErrorHandler

	mu      sync.Mutex
	nextID  uint64
	entries map[string]expiryEntry
}

func NewExpiryScheduler(parent context.Context, service *StatusService, onExpired func(domain.Transition), onError ExpiryErrorHandler) *ExpiryScheduler {
	ctx, cancel := context.WithCancel(parent)
	return &ExpiryScheduler{
		service: service, ctx: ctx, cancel: cancel,
		onExpired: onExpired, onError: onError,
		entries: make(map[string]expiryEntry),
	}
}

func (s *ExpiryScheduler) Schedule(state domain.State) {
	key := state.RoomID + "\x00" + state.UserID
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.entries[key]; ok {
		previous.timer.Stop()
		delete(s.entries, key)
	}
	if state.ExpiresAt.Year() >= 9999 || s.ctx.Err() != nil {
		return
	}

	s.nextID++
	generation := s.nextID
	delay := time.Until(state.ExpiresAt)
	if delay < 0 {
		delay = 0
	}
	// A zero-duration timer may execute before AfterFunc returns. Gate the
	// callback until the map entry has been installed, otherwise an immediate
	// expiry could observe no entry and silently remain uncleared.
	registered := make(chan struct{})
	timer := time.AfterFunc(delay, func() {
		<-registered
		s.run(key, generation, state)
	})
	s.entries[key] = expiryEntry{generation: generation, timer: timer}
	close(registered)
}

func (s *ExpiryScheduler) run(key string, generation uint64, state domain.State) {
	s.mu.Lock()
	entry, ok := s.entries[key]
	if !ok || entry.generation != generation {
		s.mu.Unlock()
		return
	}
	delete(s.entries, key)
	s.mu.Unlock()

	if s.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	transition, cleared, err := s.service.Expire(ctx, state, time.Now().UTC())
	if err != nil {
		if s.onError != nil {
			s.onError(err, state)
		}
		return
	}
	if cleared && s.onExpired != nil {
		s.onExpired(transition)
	}
}

func (s *ExpiryScheduler) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.entries {
		entry.timer.Stop()
		delete(s.entries, key)
	}
}
