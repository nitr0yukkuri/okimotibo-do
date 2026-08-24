package api

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"
	"strings"
	"sync"
	"time"
)

const pairingTTL = 10 * time.Minute
const pairingCodeLength = 8

const pairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type pairingGrant struct {
	RoomID    string
	UserID    string
	ExpiresAt time.Time
}

type pairingStore struct {
	mu     sync.Mutex
	grants map[string]pairingGrant
	codes  map[string]string
}

func newPairingStore() *pairingStore {
	return &pairingStore{grants: make(map[string]pairingGrant), codes: make(map[string]string)}
}

func (s *pairingStore) issue(roomID, userID string, now time.Time) (string, string, pairingGrant, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", pairingGrant{}, err
	}
	token := "pair_" + base64.RawURLEncoding.EncodeToString(bytes)
	code, err := newPairingCode()
	if err != nil {
		return "", "", pairingGrant{}, err
	}
	grant := pairingGrant{RoomID: roomID, UserID: userID, ExpiresAt: now.Add(pairingTTL)}
	s.mu.Lock()
	s.grants[token] = grant
	s.codes[code] = token
	s.mu.Unlock()
	return token, code, grant, nil
}

func newPairingCode() (string, error) {
	var builder strings.Builder
	for i := 0; i < pairingCodeLength; i++ {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(pairingCodeAlphabet))))
		if err != nil {
			return "", err
		}
		builder.WriteByte(pairingCodeAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

func normalizePairingCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

func (s *pairingStore) claim(code string, now time.Time) (string, pairingGrant, bool) {
	code = normalizePairingCode(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.codes[code]
	if !ok {
		return "", pairingGrant{}, false
	}
	grant, ok := s.grants[token]
	if !ok || !grant.ExpiresAt.After(now) {
		delete(s.codes, code)
		delete(s.grants, token)
		return "", pairingGrant{}, false
	}
	// The human-entered code is single-use. The issued token remains valid
	// until its normal expiry, just like a QR token.
	delete(s.codes, code)
	return token, grant, true
}

func (s *pairingStore) lookup(token string, now time.Time) (pairingGrant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.grants[token]
	if !ok {
		return pairingGrant{}, false
	}
	if !grant.ExpiresAt.After(now) {
		delete(s.grants, token)
		return pairingGrant{}, false
	}
	return grant, true
}
