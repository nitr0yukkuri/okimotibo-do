package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

type Supabase struct {
	baseURL string
	key     string
	client  *http.Client
}

func NewSupabase(baseURL, key string, timeout time.Duration) *Supabase {
	return &Supabase{baseURL: baseURL, key: key, client: &http.Client{Timeout: timeout}}
}

func (s *Supabase) Authenticate(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", errors.New("access token is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/auth/v1/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("apikey", s.key)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("authenticate user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("invalid access token")
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&user); err != nil || user.ID == "" {
		return "", errors.New("invalid authentication response")
	}
	return user.ID, nil
}

func (s *Supabase) AuthorizeRoom(ctx context.Context, userID, roomID string) error {
	endpoint := fmt.Sprintf("%s/rest/v1/room_members?room_id=eq.%s&user_id=eq.%s&select=user_id&limit=1",
		s.baseURL, url.QueryEscape(roomID), url.QueryEscape(userID))
	var rows []struct {
		UserID string `json:"user_id"`
	}
	if err := s.requestJSON(ctx, http.MethodGet, endpoint, nil, &rows, ""); err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("user is not a room member")
	}
	return nil
}

func (s *Supabase) UpsertState(ctx context.Context, state domain.State) error {
	payload := map[string]any{
		"p_user_id": state.UserID, "p_room_id": state.RoomID, "p_client_id": state.ClientID,
		"p_sequence": state.Sequence, "p_status": state.Status, "p_captured_at": state.CapturedAt,
		"p_received_at": state.ReceivedAt, "p_expires_at": state.ExpiresAt, "p_hand": state.Hand, "p_face": state.Face, "p_source": state.Source,
	}
	var updated bool
	err := s.requestJSON(ctx, http.MethodPost, s.baseURL+"/rest/v1/rpc/upsert_current_status", payload, &updated, "")
	if err != nil {
		return err
	}
	if !updated {
		return ErrStaleState
	}
	return nil
}

func (s *Supabase) GetState(ctx context.Context, roomID, userID string) (domain.State, error) {
	endpoint := s.baseURL + "/rest/v1/current_statuses?room_id=eq." + url.QueryEscape(roomID) + "&user_id=eq." + url.QueryEscape(userID) + "&expires_at=gt." + url.QueryEscape(time.Now().UTC().Format(time.RFC3339Nano)) + "&select=*&limit=1"
	var rows []struct {
		RoomID     string        `json:"room_id"`
		UserID     string        `json:"user_id"`
		ClientID   string        `json:"client_id"`
		Sequence   uint64        `json:"sequence"`
		Status     domain.Status `json:"status"`
		CapturedAt time.Time     `json:"captured_at"`
		ReceivedAt time.Time     `json:"received_at"`
		ExpiresAt  time.Time     `json:"expires_at"`
		Hand       *domain.Hand  `json:"hand"`
		Face       *domain.Face  `json:"face"`
		Source     domain.Source `json:"source"`
	}
	if err := s.requestJSON(ctx, http.MethodGet, endpoint, nil, &rows, ""); err != nil {
		return domain.State{}, err
	}
	if len(rows) == 0 {
		return domain.State{}, ErrNotFound
	}
	r := rows[0]
	return domain.State{RoomID: r.RoomID, UserID: r.UserID, ClientID: r.ClientID, Sequence: r.Sequence,
		Status: r.Status, CapturedAt: r.CapturedAt, ReceivedAt: r.ReceivedAt, ExpiresAt: r.ExpiresAt, Hand: r.Hand, Face: r.Face, Source: r.Source}, nil
}

func (s *Supabase) ClearState(ctx context.Context, roomID, userID, clientID string) (bool, error) {
	endpoint := s.baseURL + "/rest/v1/current_statuses?room_id=eq." + url.QueryEscape(roomID) +
		"&user_id=eq." + url.QueryEscape(userID) + "&client_id=eq." + url.QueryEscape(clientID)
	var rows []struct {
		RoomID string `json:"room_id"`
	}
	if err := s.requestJSON(ctx, http.MethodDelete, endpoint, nil, &rows, "return=representation"); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (s *Supabase) ClearStateIfCurrent(ctx context.Context, roomID, userID, clientID string, sequence uint64, receivedAt time.Time) (bool, error) {
	endpoint := s.baseURL + "/rest/v1/current_statuses?room_id=eq." + url.QueryEscape(roomID) +
		"&user_id=eq." + url.QueryEscape(userID) +
		"&client_id=eq." + url.QueryEscape(clientID) +
		"&sequence=eq." + url.QueryEscape(fmt.Sprint(sequence)) +
		"&received_at=eq." + url.QueryEscape(receivedAt.Format(time.RFC3339Nano))
	var rows []struct {
		RoomID string `json:"room_id"`
	}
	if err := s.requestJSON(ctx, http.MethodDelete, endpoint, nil, &rows, "return=representation"); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (s *Supabase) RefreshManualLeaseIfCurrent(ctx context.Context, current domain.State, capturedAt, receivedAt, expiresAt time.Time) (bool, error) {
	endpoint := s.baseURL + "/rest/v1/current_statuses?room_id=eq." + url.QueryEscape(current.RoomID) +
		"&user_id=eq." + url.QueryEscape(current.UserID) +
		"&client_id=eq." + url.QueryEscape(current.ClientID) +
		"&sequence=eq." + url.QueryEscape(fmt.Sprint(current.Sequence)) +
		"&received_at=eq." + url.QueryEscape(current.ReceivedAt.Format(time.RFC3339Nano)) +
		"&source=eq.manual&expires_at=gt." + url.QueryEscape(receivedAt.Format(time.RFC3339Nano))
	payload := map[string]any{
		"captured_at": capturedAt,
		"received_at": receivedAt,
		"expires_at":  expiresAt,
	}
	var rows []struct {
		RoomID string `json:"room_id"`
	}
	if err := s.requestJSON(ctx, http.MethodPatch, endpoint, payload, &rows, "return=representation"); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (s *Supabase) requestJSON(ctx context.Context, method, endpoint string, body any, target any, prefer string) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", s.key)
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("supabase returned %d: %s", resp.StatusCode, message)
	}
	if target != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(target)
	}
	return nil
}
