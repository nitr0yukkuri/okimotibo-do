package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"math/big"
	"strings"
	"time"
)

var ErrPairingUnavailable = errors.New("pairing storage unavailable")

const (
	pairingCodeLength   = 8
	pairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

type PairingGrant struct {
	Token     string
	Code      string
	RoomID    string
	UserID    string
	ExpiresAt time.Time
}

type PairingRepository interface {
	CreatePairing(ctx context.Context, roomID, userID string, expiresAt time.Time) (PairingGrant, error)
	ClaimPairing(ctx context.Context, code string, now time.Time) (PairingGrant, bool, error)
	GetPairing(ctx context.Context, token string, now time.Time) (PairingGrant, bool, error)
}

func newPairingCredentials() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	token := "pair_" + base64.RawURLEncoding.EncodeToString(bytes)
	var builder strings.Builder
	for i := 0; i < pairingCodeLength; i++ {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(pairingCodeAlphabet))))
		if err != nil {
			return "", "", err
		}
		builder.WriteByte(pairingCodeAlphabet[index.Int64()])
	}
	return token, builder.String(), nil
}

func normalizePairingCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

func (m *Memory) CreatePairing(_ context.Context, roomID, userID string, expiresAt time.Time) (PairingGrant, error) {
	for attempt := 0; attempt < 3; attempt++ {
		token, code, err := newPairingCredentials()
		if err != nil {
			return PairingGrant{}, err
		}
		grant := PairingGrant{Token: token, Code: code, RoomID: roomID, UserID: userID, ExpiresAt: expiresAt}
		m.mu.Lock()
		if _, tokenExists := m.pairings[token]; tokenExists {
			m.mu.Unlock()
			continue
		}
		if _, codeExists := m.pairingCodes[code]; codeExists {
			m.mu.Unlock()
			continue
		}
		m.pairings[token] = grant
		m.pairingCodes[code] = token
		m.mu.Unlock()
		return grant, nil
	}
	return PairingGrant{}, errors.New("pairing credential collision")
}

func (m *Memory) ClaimPairing(_ context.Context, code string, now time.Time) (PairingGrant, bool, error) {
	code = normalizePairingCode(code)
	m.mu.Lock()
	defer m.mu.Unlock()
	token, ok := m.pairingCodes[code]
	if !ok {
		return PairingGrant{}, false, nil
	}
	grant, ok := m.pairings[token]
	if !ok || !grant.ExpiresAt.After(now) {
		delete(m.pairingCodes, code)
		delete(m.pairings, token)
		return PairingGrant{}, false, nil
	}
	delete(m.pairingCodes, code)
	return grant, true, nil
}

func (m *Memory) GetPairing(_ context.Context, token string, now time.Time) (PairingGrant, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	grant, ok := m.pairings[token]
	if !ok {
		return PairingGrant{}, false, nil
	}
	if !grant.ExpiresAt.After(now) {
		delete(m.pairings, token)
		delete(m.pairingCodes, grant.Code)
		return PairingGrant{}, false, nil
	}
	return grant, true, nil
}
