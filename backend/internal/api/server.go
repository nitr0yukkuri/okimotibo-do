package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/config"
	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/store"
	"github.com/coder/websocket"
)

const maxMessageBytes = 16 << 10

type Server struct {
	cfg    config.Config
	store  store.Repository
	hub    *hub
	logger *slog.Logger
}

type client struct {
	conn                     *websocket.Conn
	roomID, userID, clientID string
	send                     chan []byte
	cancel                   context.CancelFunc
	lastSequence             uint64
}

func NewServer(cfg config.Config, repository store.Repository, logger *slog.Logger) *Server {
	if cfg.StatusTTL <= 0 {
		cfg.StatusTTL = 15 * time.Minute
	}
	return &Server{cfg: cfg, store: repository, hub: newHub(), logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/status/{userID}", s.getStatus)
	mux.HandleFunc("GET /api/v1/ws", s.websocket)
	return s.recover(s.cors(mux))
}

func (s *Server) Close() { s.hub.close() }

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
		}
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	authCtx, authCancel := context.WithTimeout(ctx, 8*time.Second)
	messageKind, data, err := conn.Read(authCtx)
	authCancel()
	if err != nil {
		s.logger.Warn("client.hello not received", "error", err)
		conn.Close(websocket.StatusPolicyViolation, "client.hello required")
		return
	}
	var hello helloMessage
	if messageKind != websocket.MessageText || decodeStrict(data, &hello) != nil || hello.Type != "client.hello" || !validID(hello.RoomID) || !validID(hello.ClientID) {
		s.logger.Warn("invalid client.hello", "messageKind", messageKind)
		conn.Close(websocket.StatusPolicyViolation, "invalid client.hello")
		return
	}

	userID := hello.UserID
	if s.cfg.AllowAnonymous {
		if !validID(userID) {
			s.logger.Warn("anonymous userId missing or invalid")
			conn.Close(websocket.StatusPolicyViolation, "userId required")
			return
		}
	} else {
		userID, err = s.store.Authenticate(ctx, hello.Token)
		if err != nil {
			s.logger.Warn("authentication failed", "error", err)
			conn.Close(websocket.StatusPolicyViolation, "authentication failed")
			return
		}
	}
	if err := s.store.AuthorizeRoom(ctx, userID, hello.RoomID); err != nil {
		s.logger.Warn("room access denied", "error", err, "roomId", hello.RoomID, "userId", userID)
		conn.Close(websocket.StatusPolicyViolation, "room access denied")
		return
	}
	c.roomID, c.userID, c.clientID = hello.RoomID, userID, hello.ClientID
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
		if kind != "recognition.update" {
			s.sendError(c, "unknown_type", "unsupported message type")
			continue
		}
		var incoming recognitionMessage
		if decodeStrict(data, &incoming) != nil {
			s.sendError(c, "invalid_message", "invalid recognition.update")
			continue
		}
		now := time.Now().UTC()
		state := domain.State{RoomID: c.roomID, UserID: c.userID, ClientID: c.clientID, Sequence: incoming.Sequence,
			CapturedAt: incoming.CapturedAt, ReceivedAt: now, ExpiresAt: now.Add(s.cfg.StatusTTL), Status: incoming.Status, Hand: incoming.Hand, Face: incoming.Face, Source: incoming.Source}
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
		message, _ := json.Marshal(outgoingMessage{Type: "status.changed", State: &state})
		s.hub.broadcast(c.roomID, message)
	}
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
	if !s.cfg.AllowAnonymous {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
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
