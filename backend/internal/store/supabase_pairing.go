package store

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

type pairingRow struct {
	Token     string    `json:"token"`
	Code      string    `json:"code"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Supabase) CreatePairing(ctx context.Context, roomID, userID string, expiresAt time.Time) (PairingGrant, error) {
	for attempt := 0; attempt < 3; attempt++ {
		token, code, err := newPairingCredentials()
		if err != nil {
			return PairingGrant{}, err
		}
		payload := map[string]any{
			"token": token, "code": code, "room_id": roomID, "user_id": userID, "expires_at": expiresAt,
		}
		var rows []pairingRow
		err = s.requestJSON(ctx, http.MethodPost, s.baseURL+"/rest/v1/pairing_grants", payload, &rows, "return=representation")
		if err == nil && len(rows) == 1 {
			return pairingGrantFromRow(rows[0]), nil
		}
		if err != nil && attempt == 2 {
			return PairingGrant{}, err
		}
	}
	return PairingGrant{}, ErrPairingUnavailable
}

func (s *Supabase) ClaimPairing(ctx context.Context, code string, now time.Time) (PairingGrant, bool, error) {
	endpoint := s.baseURL + "/rest/v1/pairing_grants?code=eq." + url.QueryEscape(normalizePairingCode(code)) +
		"&claimed_at=is.null&expires_at=gt." + url.QueryEscape(now.UTC().Format(time.RFC3339Nano))
	var rows []pairingRow
	err := s.requestJSON(ctx, http.MethodPatch, endpoint, map[string]any{"claimed_at": now.UTC()}, &rows, "return=representation")
	if err != nil {
		return PairingGrant{}, false, err
	}
	if len(rows) == 0 {
		return PairingGrant{}, false, nil
	}
	return pairingGrantFromRow(rows[0]), true, nil
}

func (s *Supabase) GetPairing(ctx context.Context, token string, now time.Time) (PairingGrant, bool, error) {
	endpoint := s.baseURL + "/rest/v1/pairing_grants?token=eq." + url.QueryEscape(token) +
		"&expires_at=gt." + url.QueryEscape(now.UTC().Format(time.RFC3339Nano)) +
		"&select=token,code,room_id,user_id,expires_at&limit=1"
	var rows []pairingRow
	if err := s.requestJSON(ctx, http.MethodGet, endpoint, nil, &rows, ""); err != nil {
		return PairingGrant{}, false, err
	}
	if len(rows) == 0 {
		return PairingGrant{}, false, nil
	}
	return pairingGrantFromRow(rows[0]), true, nil
}

func pairingGrantFromRow(row pairingRow) PairingGrant {
	return PairingGrant{Token: row.Token, Code: row.Code, RoomID: row.RoomID, UserID: row.UserID, ExpiresAt: row.ExpiresAt}
}
