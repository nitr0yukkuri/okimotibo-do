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

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
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

func TestSupabaseClearStateFiltersByClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/current_statuses" || r.Method != http.MethodDelete {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		query := r.URL.RawQuery
		if !strings.Contains(query, "room_id=eq.room-1") || !strings.Contains(query, "user_id=eq.user-1") || !strings.Contains(query, "client_id=eq.client-1") {
			t.Errorf("clear request is not scoped to the client: %s", query)
		}
		if r.Header.Get("Prefer") != "return=representation" {
			t.Errorf("clear request must request deleted rows: Prefer=%q", r.Header.Get("Prefer"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"room_id":"room-1"}]`))
	}))
	defer server.Close()

	repository := NewSupabase(server.URL, "service-key", time.Second)
	cleared, err := repository.ClearState(context.Background(), "room-1", "user-1", "client-1")
	if err != nil || !cleared {
		t.Fatalf("expected Supabase state to be cleared: cleared=%v err=%v", cleared, err)
	}
}

func TestSupabaseConditionalClearFiltersVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/current_statuses" || r.Method != http.MethodDelete {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		query := r.URL.Query()
		if query.Get("client_id") != "eq.client-1" || query.Get("sequence") != "eq.7" || query.Get("received_at") != "eq.2026-08-30T00:00:00Z" {
			t.Errorf("conditional clear filters are missing: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"room_id":"room-1"}]`))
	}))
	defer server.Close()

	repository := NewSupabase(server.URL, "service-key", time.Second)
	cleared, err := repository.ClearStateIfCurrent(context.Background(), "room-1", "user-1", "client-1", 7, time.Date(2026, time.August, 30, 0, 0, 0, 0, time.UTC))
	if err != nil || !cleared {
		t.Fatalf("expected conditional Supabase clear: cleared=%v err=%v", cleared, err)
	}
}

func TestSupabaseConditionalLeaseRefreshFiltersVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/current_statuses" || r.Method != http.MethodPatch {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		query := r.URL.Query()
		if query.Get("client_id") != "eq.client-1" || query.Get("sequence") != "eq.7" || query.Get("received_at") != "eq.2026-08-30T00:00:00Z" || query.Get("source") != "eq.manual" {
			t.Errorf("conditional lease filters are missing: %s", r.URL.RawQuery)
		}
		var payload map[string]time.Time
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["received_at"].IsZero() || payload["expires_at"].IsZero() {
			t.Fatalf("lease timestamps are missing: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"room_id":"room-1"}]`))
	}))
	defer server.Close()

	repository := NewSupabase(server.URL, "service-key", time.Second)
	current := domain.State{RoomID: "room-1", UserID: "user-1", ClientID: "client-1", Sequence: 7, ReceivedAt: time.Date(2026, time.August, 30, 0, 0, 0, 0, time.UTC), Source: domain.SourceManual}
	refreshed, err := repository.RefreshManualLeaseIfCurrent(context.Background(), current, time.Date(2026, time.August, 30, 0, 0, 1, 0, time.UTC), time.Date(2026, time.August, 30, 0, 0, 1, 0, time.UTC), time.Date(2026, time.August, 30, 0, 1, 1, 0, time.UTC))
	if err != nil || !refreshed {
		t.Fatalf("expected conditional Supabase lease refresh: refreshed=%v err=%v", refreshed, err)
	}
}
