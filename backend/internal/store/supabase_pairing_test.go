package store

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSupabasePairingPersistsAndClaimsOnce(t *testing.T) {
	var issued pairingRow
	claimCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/pairing_grants" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Token     string    `json:"token"`
				Code      string    `json:"code"`
				RoomID    string    `json:"room_id"`
				UserID    string    `json:"user_id"`
				ExpiresAt time.Time `json:"expires_at"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			issued = pairingRow{Token: payload.Token, Code: payload.Code, RoomID: payload.RoomID, UserID: payload.UserID, ExpiresAt: payload.ExpiresAt}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]pairingRow{issued})
		case http.MethodGet:
			if !strings.Contains(r.URL.RawQuery, "token=eq.") {
				t.Errorf("missing token filter: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]pairingRow{issued})
		case http.MethodPatch:
			if !strings.Contains(r.URL.RawQuery, "claimed_at=is.null") || !strings.Contains(r.URL.RawQuery, "expires_at=gt.") {
				t.Errorf("claim is not conditional: %s", r.URL.RawQuery)
			}
			claimCount++
			w.Header().Set("Content-Type", "application/json")
			if claimCount == 1 {
				_ = json.NewEncoder(w).Encode([]pairingRow{issued})
				return
			}
			_ = json.NewEncoder(w).Encode([]pairingRow{})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	repository := NewSupabase(server.URL, "service-key", time.Second)
	now := time.Now().UTC().Truncate(time.Millisecond)
	created, err := repository.CreatePairing(context.Background(), "room-1", "user-1", now.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.Code == "" {
		t.Fatalf("created grant is incomplete: %#v", created)
	}
	if _, found, err := repository.GetPairing(context.Background(), created.Token, now); err != nil || !found {
		t.Fatalf("get pairing: found=%v err=%v", found, err)
	}
	if _, found, err := repository.ClaimPairing(context.Background(), created.Code, now); err != nil || !found {
		t.Fatalf("claim pairing: found=%v err=%v", found, err)
	}
	if _, found, err := repository.ClaimPairing(context.Background(), created.Code, now); err != nil || found {
		t.Fatalf("claim should be single-use: found=%v err=%v", found, err)
	}
}
