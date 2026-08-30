package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/config"
	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/store"
	"github.com/coder/websocket"
)

const maxMessageBytes = 16 << 10

const disconnectGrace = 5 * time.Second

const (
	defaultManualLease = 30 * time.Second
	minManualLease     = 1 * time.Second
	maxManualLease     = 24 * time.Hour
)

type Server struct {
	cfg     config.Config
	store   store.Repository
	hub     *hub
	pairing store.PairingRepository
	logger  *slog.Logger
	ready   atomic.Bool
	lifeCtx context.Context
	stop    context.CancelFunc
	bus     *redisBus
	origin  string
}

type client struct {
	conn                     *websocket.Conn
	roomID, userID, clientID string
	readOnly                 bool
	send                     chan []byte
	cancel                   context.CancelFunc
	lastSequence             uint64
}

func NewServer(cfg config.Config, repository store.Repository, logger *slog.Logger) *Server {
	pairing, ok := repository.(store.PairingRepository)
	if !ok {
		pairing = store.NewMemory()
	}
	lifeCtx, stop := context.WithCancel(context.Background())
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "backend"
	}
	origin := fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
	server := &Server{cfg: cfg, store: repository, hub: newHub(), pairing: pairing, logger: logger, lifeCtx: lifeCtx, stop: stop, origin: origin}
	if cfg.RedisURL != "" {
		bus, err := newRedisBus(cfg.RedisURL, origin, logger)
		if err != nil {
			logger.Error("invalid REDIS_URL; cross-pod events disabled", "error", err)
		} else {
			server.bus = bus
			go bus.run(lifeCtx, server.forwardRemoteEvent)
		}
	}
	server.ready.Store(true)
	return server
}

func statusExpiresAt(now time.Time, ttl time.Duration, source domain.Source) time.Time {
	if source == domain.SourceManual || ttl <= 0 {
		return time.Date(9999, time.December, 31, 23, 59, 59, 999999999, time.UTC)
	}
	return now.Add(ttl)
}

func statusExpiresAtWithLease(now time.Time, ttl time.Duration, source domain.Source, leaseSeconds *int) (time.Time, error) {
	if leaseSeconds == nil {
		return statusExpiresAt(now, ttl, source), nil
	}
	if source != domain.SourceManual {
		return time.Time{}, errors.New("leaseSeconds is only supported for manual status")
	}
	if *leaseSeconds < int(minManualLease/time.Second) || *leaseSeconds > int(maxManualLease/time.Second) {
		return time.Time{}, errors.New("leaseSeconds must be between 1 and 86400")
	}
	lease := time.Duration(*leaseSeconds) * time.Second
	return now.Add(lease), nil
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !s.ready.Load() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "draining"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/status/{userID}", s.getStatus)
	mux.HandleFunc("POST /api/v1/pairing", s.createPairing)
	mux.HandleFunc("POST /api/v1/pairing/claim", s.claimPairing)
	mux.HandleFunc("GET /api/v1/ws", s.websocket)
	return s.recover(s.cors(mux))
}

func (s *Server) Close() {
	s.ready.Store(false)
	s.stop()
	s.hub.close()
	if s.bus != nil {
		_ = s.bus.close()
	}
}

func (s *Server) broadcast(roomID string, message []byte) {
	s.hub.broadcast(roomID, message)
	if s.bus == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.lifeCtx, 2*time.Second)
	defer cancel()
	if err := s.bus.publish(ctx, roomID, message); err != nil && s.lifeCtx.Err() == nil {
		s.logger.Warn("redis status event publish failed", "error", err, "roomId", roomID)
	}
}

func (s *Server) forwardRemoteEvent(event pubsubEnvelope) {
	var message outgoingMessage
	if event.Message != nil && json.Unmarshal(event.Message, &message) == nil && message.Type == "status.changed" && message.State != nil {
		// Every Pod schedules expiry for remote events too. If the publishing
		// Pod disappears, another Pod can still clear the shared state and
		// notify its local display clients.
		s.scheduleStateExpiry(*message.State)
	}
	s.hub.broadcast(event.RoomID, event.Message)
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r.Header.Get("Origin")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin is not allowed"})
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		s.logger.Warn("websocket rejected", "error", err)
		return
	}
	conn.SetReadLimit(maxMessageBytes)
	ctx, cancel := context.WithCancel(r.Context())
	c := &client{conn: conn, send: make(chan []byte, 16), cancel: cancel}
	defer func() {
		cancel()
		if c.roomID != "" {
			s.hub.remove(c)
			s.scheduleDisconnectedStateClear(c)
		}
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	authCtx, authCancel := context.WithTimeout(ctx, 8*time.Second)
	messageKind, data, err := conn.Read(authCtx)
	authCancel()
	if err != nil {
		conn.Close(websocket.StatusPolicyViolation, "client.hello required")
		return
	}
	var hello helloMessage
	if messageKind != websocket.MessageText || decodeStrict(data, &hello) != nil || hello.Type != "client.hello" || !validID(hello.RoomID) || !validID(hello.ClientID) {
		conn.Close(websocket.StatusPolicyViolation, "invalid client.hello")
		return
	}

	userID := hello.UserID
	readOnly := false
	if isPairingToken(hello.Token) {
		grant, pairingFound, pairingErr := s.pairing.GetPairing(ctx, hello.Token, time.Now().UTC())
		if pairingErr != nil {
			conn.Close(websocket.StatusInternalError, "pairing storage unavailable")
			return
		}
		if !pairingFound {
			conn.Close(websocket.StatusPolicyViolation, "invalid pairing token")
			return
		}
		if hello.RoomID != grant.RoomID || (hello.UserID != "" && hello.UserID != grant.UserID) {
			conn.Close(websocket.StatusPolicyViolation, "pairing scope mismatch")
			return
		}
		userID = grant.UserID
		readOnly = true
	} else if s.cfg.AllowAnonymous {
		if !validID(userID) {
			conn.Close(websocket.StatusPolicyViolation, "userId required")
			return
		}
	} else {
		userID, err = s.store.Authenticate(ctx, hello.Token)
		if err != nil {
			conn.Close(websocket.StatusPolicyViolation, "authentication failed")
			return
		}
	}
	if !s.cfg.AllowAnonymous {
		if err := s.store.AuthorizeRoom(ctx, userID, hello.RoomID); err != nil {
			conn.Close(websocket.StatusPolicyViolation, "room access denied")
			return
		}
	}
	c.roomID, c.userID, c.clientID, c.readOnly = hello.RoomID, userID, hello.ClientID, readOnly
	s.hub.add(c)
	ack, _ := json.Marshal(outgoingMessage{Type: "server.ready"})
	c.send <- ack

	go s.writeLoop(ctx, c)
	s.readLoop(ctx, c)
}

func (s *Server) readLoop(ctx context.Context, c *client) {
	for {
		messageKind, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		if messageKind != websocket.MessageText {
			s.sendError(c, "invalid_message", "only JSON text messages are accepted")
			continue
		}
		kind, err := messageType(data)
		if err != nil {
			s.sendError(c, "invalid_json", "message must be valid JSON")
			continue
		}
		if kind == "status.heartbeat" {
			s.handleHeartbeat(ctx, c, data)
			continue
		}
		if kind != "recognition.update" {
			s.sendError(c, "unknown_type", "unsupported message type")
			continue
		}
		if c.readOnly {
			s.sendError(c, "read_only", "paired clients cannot publish status")
			continue
		}
		var incoming recognitionMessage
		if decodeStrict(data, &incoming) != nil {
			s.sendError(c, "invalid_message", "invalid recognition.update")
			continue
		}
		now := time.Now().UTC()
		expiresAt, expiryErr := statusExpiresAtWithLease(now, s.cfg.StatusTTL, incoming.Source, incoming.LeaseSeconds)
		if expiryErr != nil {
			s.sendError(c, "validation_failed", expiryErr.Error())
			continue
		}
		state := domain.State{RoomID: c.roomID, UserID: c.userID, ClientID: c.clientID, Sequence: incoming.Sequence,
			CapturedAt: incoming.CapturedAt, ReceivedAt: now, ExpiresAt: expiresAt, Status: incoming.Status, Hand: incoming.Hand, Face: incoming.Face, Source: incoming.Source}
		if err := state.Validate(now); err != nil {
			s.sendError(c, "validation_failed", err.Error())
			continue
		}
		if state.Sequence <= c.lastSequence {
			s.sendError(c, "stale_sequence", "sequence must increase")
			continue
		}
		if err := s.store.UpsertState(ctx, state); err != nil {
			if errors.Is(err, store.ErrStaleState) {
				s.sendError(c, "stale_state", "a newer state is already stored")
				continue
			}
			s.logger.Error("state persistence failed", "error", err)
			s.sendError(c, "persistence_failed", "state was not saved")
			continue
		}
		c.lastSequence = state.Sequence
		s.scheduleStateExpiry(state)
		message, _ := json.Marshal(outgoingMessage{Type: "status.changed", State: &state})
		s.broadcast(c.roomID, message)
	}
}

func (s *Server) handleHeartbeat(ctx context.Context, c *client, data []byte) {
	if c.readOnly {
		s.sendError(c, "read_only", "paired clients cannot publish status")
		return
	}
	var heartbeat heartbeatMessage
	if decodeStrict(data, &heartbeat) != nil || heartbeat.Type != "status.heartbeat" {
		s.sendError(c, "invalid_message", "invalid status.heartbeat")
		return
	}
	leaseSeconds := int(defaultManualLease / time.Second)
	if heartbeat.LeaseSeconds != nil {
		leaseSeconds = *heartbeat.LeaseSeconds
	}
	now := time.Now().UTC()
	expiresAt, err := statusExpiresAtWithLease(now, s.cfg.StatusTTL, domain.SourceManual, &leaseSeconds)
	if err != nil {
		s.sendError(c, "validation_failed", err.Error())
		return
	}
	current, err := s.store.GetState(ctx, c.roomID, c.userID)
	if errors.Is(err, store.ErrNotFound) {
		s.sendError(c, "state_expired", "no active manual lease to refresh")
		return
	}
	if err != nil {
		s.sendError(c, "persistence_failed", "state was not loaded")
		return
	}
	if current.ClientID != c.clientID || current.Source != domain.SourceManual {
		s.sendError(c, "lease_owner_mismatch", "only the manual state owner can refresh its lease")
		return
	}
	refresher, ok := s.store.(store.ConditionalLeaseRefresher)
	if !ok {
		s.logger.Error("manual lease refresh is unsupported by the state store")
		s.sendError(c, "persistence_failed", "lease refresh is not supported")
		return
	}
	refreshed, err := refresher.RefreshManualLeaseIfCurrent(ctx, current, now, now, expiresAt)
	if err != nil {
		s.sendError(c, "persistence_failed", "lease was not refreshed")
		return
	}
	if !refreshed {
		s.sendError(c, "stale_state", "a newer state is already stored")
		return
	}
	current.CapturedAt = now
	current.ReceivedAt = now
	current.ExpiresAt = expiresAt
	s.scheduleStateExpiry(current)
	message, _ := json.Marshal(outgoingMessage{Type: "status.changed", State: &current})
	s.broadcast(c.roomID, message)
}

func (s *Server) scheduleStateExpiry(state domain.State) {
	if state.ExpiresAt.Year() >= 9999 {
		return
	}
	delay := time.Until(state.ExpiresAt)
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		if s.lifeCtx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.lifeCtx, 5*time.Second)
		defer cancel()
		cleared, err := s.clearStateIfCurrent(ctx, state)
		if err != nil {
			s.logger.Warn("failed to clear expired status", "error", err, "roomId", state.RoomID, "userId", state.UserID)
			return
		}
		if cleared {
			s.broadcastCleared(state)
		}
	})
}

func (s *Server) clearStateIfCurrent(ctx context.Context, state domain.State) (bool, error) {
	if clearer, ok := s.store.(store.ConditionalClearer); ok {
		return clearer.ClearStateIfCurrent(ctx, state.RoomID, state.UserID, state.ClientID, state.Sequence, state.ReceivedAt)
	}
	return s.store.ClearState(ctx, state.RoomID, state.UserID, state.ClientID)
}

func (s *Server) broadcastCleared(state domain.State) {
	message, _ := json.Marshal(outgoingMessage{
		Type: "status.cleared", RoomID: state.RoomID, UserID: state.UserID, ClientID: state.ClientID,
		CapturedAt: state.CapturedAt, ReceivedAt: state.ReceivedAt,
	})
	s.broadcast(state.RoomID, message)
}

func (s *Server) scheduleDisconnectedStateClear(c *client) {
	if c.readOnly || c.roomID == "" || c.userID == "" || c.clientID == "" {
		return
	}
	time.AfterFunc(disconnectGrace, func() {
		if s.hub.hasPublisher(c.roomID, c.userID) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		current, err := s.store.GetState(ctx, c.roomID, c.userID)
		if errors.Is(err, store.ErrNotFound) {
			return
		}
		if err != nil {
			s.logger.Warn("failed to inspect disconnected status", "error", err, "roomId", c.roomID, "userId", c.userID)
			return
		}
		// A publisher may have connected while the database was being read.
		// Re-check immediately before the conditional delete so a reconnecting
		// PC is not cleared because of the first check's stale snapshot.
		if s.hub.hasPublisher(c.roomID, c.userID) {
			return
		}
		// Delete only the state that was current when there were no publishers.
		// The conditional store operation also compares sequence and receivedAt,
		// so a newer state from the same client cannot be removed by this timer.
		cleared, err := s.clearStateIfCurrent(ctx, current)
		if err != nil {
			s.logger.Warn("failed to clear disconnected status", "error", err, "roomId", c.roomID, "userId", c.userID, "clientId", c.clientID)
			return
		}
		if !cleared {
			return
		}
		s.broadcastCleared(current)
	})
}

func (s *Server) writeLoop(ctx context.Context, c *client) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-c.send:
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Write(writeCtx, websocket.MessageText, message)
			cancel()
			if err != nil {
				c.cancel()
				return
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				c.cancel()
				return
			}
		}
	}
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("roomID")
	userID := r.PathValue("userID")
	if !validID(roomID) || !validID(userID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid room or user id"})
		return
	}
	token, _ := bearerToken(r.Header.Get("Authorization"))
	if isPairingToken(token) {
		grant, pairingFound, pairingErr := s.pairing.GetPairing(r.Context(), token, time.Now().UTC())
		if pairingErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "pairing storage unavailable"})
			return
		}
		if !pairingFound {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid pairing token"})
			return
		}
		if grant.RoomID != roomID || grant.UserID != userID {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid pairing token"})
			return
		}
	} else if !s.cfg.AllowAnonymous {
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		requester, authErr := s.store.Authenticate(r.Context(), token)
		if authErr != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		if authErr = s.store.AuthorizeRoom(r.Context(), requester, roomID); authErr != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "room access denied"})
			return
		}
	}
	state, err := s.store.GetState(r.Context(), roomID, userID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "storage unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) createPairing(w http.ResponseWriter, r *http.Request) {
	token, authenticated := bearerToken(r.Header.Get("Authorization"))
	var request struct {
		RoomID string `json:"roomId"`
		UserID string `json:"userId"`
	}
	if decodeStrictReader(r, &request) != nil || !validID(request.RoomID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid roomId"})
		return
	}
	userID := ""
	if authenticated && token != "" {
		var err error
		userID, err = s.store.Authenticate(r.Context(), token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		if err := s.store.AuthorizeRoom(r.Context(), userID, request.RoomID); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "room access denied"})
			return
		}
	} else {
		if !s.cfg.AllowAnonymous || !validID(request.UserID) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		userID = request.UserID
	}
	grant, err := s.pairing.CreatePairing(r.Context(), request.RoomID, userID, time.Now().UTC().Add(pairingTTL))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pairing token unavailable"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":     grant.Token,
		"code":      grant.Code,
		"roomId":    grant.RoomID,
		"userId":    grant.UserID,
		"expiresAt": grant.ExpiresAt,
	})
}

func (s *Server) claimPairing(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Code string `json:"code"`
	}
	if decodeStrictReader(r, &request) != nil || len(normalizePairingCode(request.Code)) != pairingCodeLength {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pairing code"})
		return
	}
	grant, ok, err := s.pairing.ClaimPairing(r.Context(), normalizePairingCode(request.Code), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "pairing storage unavailable"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pairing code is invalid or expired"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":     grant.Token,
		"roomId":    grant.RoomID,
		"userId":    grant.UserID,
		"expiresAt": grant.ExpiresAt,
	})
}
func bearerToken(header string) (string, bool) {
	token, ok := strings.CutPrefix(header, "Bearer ")
	return token, ok
}

func decodeStrictReader(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxMessageBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON value")
	}
	return nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("message must contain exactly one JSON value")
	}
	return nil
}

func (s *Server) sendError(c *client, code, message string) {
	data, _ := json.Marshal(outgoingMessage{Type: "error", Code: code, Message: message})
	select {
	case c.send <- data:
	default:
		c.cancel()
	}
}

func validID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}

func isPairingToken(value string) bool {
	return strings.HasPrefix(value, "pair_")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, allowed := range s.cfg.AllowedOrigins {
			if origin != "" && (allowed == "*" || strings.EqualFold(origin, allowed)) {
				if allowed == "*" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				break
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin string) bool {
	// Non-browser clients do not send Origin and still authenticate in client.hello.
	if origin == "" {
		return true
	}
	for _, allowed := range s.cfg.AllowedOrigins {
		if allowed == "*" || strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				s.logger.Error("panic recovered", "value", value)
				writeJSON(w, 500, map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
