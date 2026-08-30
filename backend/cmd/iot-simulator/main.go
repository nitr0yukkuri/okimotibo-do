package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
	"github.com/coder/websocket"
)

type simulatorConfig struct {
	serverURL string
	token     string
	roomID    string
	userID    string
	clientID  string
}

type helloMessage struct {
	Type     string `json:"type"`
	Token    string `json:"token"`
	RoomID   string `json:"roomId"`
	ClientID string `json:"clientId"`
	UserID   string `json:"userId,omitempty"`
}

type statusUpdate struct {
	Type       string        `json:"type"`
	Sequence   uint64        `json:"sequence"`
	CapturedAt time.Time     `json:"capturedAt"`
	Status     domain.Status `json:"status"`
	Source     domain.Source `json:"source"`
}

type serverMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	State   *struct {
		Status domain.Status `json:"status"`
	} `json:"state,omitempty"`
}

type activeConnection struct {
	conn   *websocket.Conn
	cancel context.CancelFunc
}

type connectionEvent struct {
	conn    *websocket.Conn
	message []byte
	err     error
}

type simulator struct {
	cfg        simulatorConfig
	connection *activeConnection
	sequence   uint64
	pending    *statusUpdate
	events     chan connectionEvent
}

func newSimulator(cfg simulatorConfig) *simulator {
	return &simulator{
		cfg:    cfg,
		events: make(chan connectionEvent, 16),
	}
}

func (s *simulator) connect() error {
	if s.connection != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, s.cfg.serverURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket接続に失敗: %w", err)
	}

	hello := helloMessage{
		Type:     "client.hello",
		Token:    s.cfg.token,
		RoomID:   s.cfg.roomID,
		ClientID: s.cfg.clientID,
	}
	if hello.Token == "" {
		hello.UserID = s.cfg.userID
	}
	if err := writeJSON(ctx, conn, hello); err != nil {
		conn.CloseNow()
		return fmt.Errorf("client.helloの送信に失敗: %w", err)
	}

	messageKind, data, err := conn.Read(ctx)
	if err != nil {
		conn.CloseNow()
		return fmt.Errorf("server.readyの受信に失敗: %w", err)
	}
	if messageKind != websocket.MessageText {
		conn.CloseNow()
		return fmt.Errorf("server.readyがテキストではありません")
	}
	var ready serverMessage
	if err := json.Unmarshal(data, &ready); err != nil || ready.Type != "server.ready" {
		conn.CloseNow()
		return fmt.Errorf("認証に失敗しました: %s", strings.TrimSpace(string(data)))
	}

	readCtx, readCancel := context.WithCancel(context.Background())
	s.connection = &activeConnection{conn: conn, cancel: readCancel}
	go s.readLoop(readCtx, conn)
	fmt.Printf("接続しました: %s / room=%s / user=%s\n", s.cfg.serverURL, s.cfg.roomID, s.cfg.userID)

	if s.pending != nil {
		pending := *s.pending
		if err := s.writeUpdate(pending); err != nil {
			s.disconnect()
			return fmt.Errorf("保留中の状態送信に失敗: %w", err)
		}
		s.pending = nil
		fmt.Printf("保留していた状態を送信: %s (#%d)\n", pending.Status, pending.Sequence)
	}
	return nil
}

func (s *simulator) disconnect() {
	if s.connection == nil {
		return
	}
	connection := s.connection
	s.connection = nil
	connection.cancel()
	connection.conn.CloseNow()
	fmt.Println("切断しました")
}

func (s *simulator) readLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		messageKind, data, err := conn.Read(ctx)
		if err != nil {
			select {
			case s.events <- connectionEvent{conn: conn, err: err}:
			case <-ctx.Done():
			}
			return
		}
		if messageKind != websocket.MessageText {
			continue
		}
		select {
		case s.events <- connectionEvent{conn: conn, message: data}:
		case <-ctx.Done():
			return
		}
	}
}

func (s *simulator) writeUpdate(update statusUpdate) error {
	if s.connection == nil {
		return fmt.Errorf("接続されていません")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return writeJSON(ctx, s.connection.conn, update)
}

func (s *simulator) publish(status domain.Status) {
	s.sequence++
	update := statusUpdate{
		Type:       "recognition.update",
		Sequence:   s.sequence,
		CapturedAt: time.Now().UTC(),
		Status:     status,
		Source:     domain.SourceManual,
	}

	if s.connection == nil {
		s.pending = &update
		fmt.Printf("未接続のため最新状態を保留: %s (#%d)\n", status, update.Sequence)
		return
	}
	if err := s.writeUpdate(update); err != nil {
		s.pending = &update
		s.disconnect()
		fmt.Printf("送信失敗。最新状態を保留: %v\n", err)
		return
	}
	fmt.Printf("送信: %s (#%d)\n", status, update.Sequence)
}

func (s *simulator) run() error {
	if err := s.connect(); err != nil {
		fmt.Printf("初回接続できません: %v\n", err)
		fmt.Println("Backendを起動してから r または reconnect を入力してください")
	}

	printHelp()
	commands := make(chan string)
	go readCommands(commands)

	for {
		select {
		case event := <-s.events:
			if s.connection == nil || s.connection.conn != event.conn {
				continue
			}
			if event.err != nil {
				connection := s.connection
				s.connection = nil
				connection.cancel()
				connection.conn.CloseNow()
				fmt.Printf("接続が切れました: %v\n", event.err)
			} else {
				// status.changedやerrorは接続が正常なまま届くメッセージ。
				// 正常な応答を受け取っただけで切断扱いにしない。
				printServerMessage(event.message)
			}
		case command, ok := <-commands:
			if !ok {
				s.disconnect()
				return nil
			}
			if done, err := s.handleCommand(command); done {
				s.disconnect()
				return err
			}
		}
	}
}

func (s *simulator) handleCommand(command string) (bool, error) {
	command = strings.TrimSpace(strings.ToLower(command))
	switch command {
	case "", "help", "h":
		printHelp()
	case "b", "busy":
		s.publish(domain.StatusBusy)
	case "n", "neutral":
		s.publish(domain.StatusNeutral)
	case "a", "available":
		s.publish(domain.StatusAvailable)
	case "d", "disconnect":
		s.disconnect()
	case "r", "reconnect":
		if s.connection != nil {
			s.disconnect()
		}
		if err := s.connect(); err != nil {
			fmt.Printf("再接続できません: %v\n", err)
		}
	case "q", "quit", "exit":
		return true, nil
	default:
		if strings.HasPrefix(command, "status ") {
			status, ok := parseStatus(strings.TrimSpace(strings.TrimPrefix(command, "status ")))
			if !ok {
				fmt.Println("statusは busy / neutral / available のいずれかです")
				return false, nil
			}
			s.publish(status)
			return false, nil
		}
		fmt.Println("不明なコマンドです。help と入力してください")
	}
	return false, nil
}

func readCommands(commands chan<- string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		commands <- scanner.Text()
	}
	close(commands)
}

func printHelp() {
	fmt.Println("コマンド: b=集中中 / n=話しかけてOK / a=対応可能 / d=切断 / r=再接続 / q=終了")
}

func printServerMessage(data []byte) {
	var message serverMessage
	if err := json.Unmarshal(data, &message); err != nil {
		fmt.Printf("Backendから受信: %s\n", strings.TrimSpace(string(data)))
		return
	}
	if message.Type == "status.changed" && message.State != nil {
		fmt.Printf("状態反映: %s\n", message.State.Status)
		return
	}
	if message.Type == "error" {
		fmt.Printf("Backendエラー: %s (%s)\n", message.Message, message.Code)
		return
	}
	fmt.Printf("Backendから受信: %s\n", strings.TrimSpace(string(data)))
}

func writeJSON(ctx context.Context, conn *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func parseStatus(value string) (domain.Status, bool) {
	switch value {
	case string(domain.StatusBusy):
		return domain.StatusBusy, true
	case string(domain.StatusNeutral):
		return domain.StatusNeutral, true
	case string(domain.StatusAvailable):
		return domain.StatusAvailable, true
	default:
		return "", false
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func websocketURL(value string) string {
	switch {
	case strings.HasPrefix(value, "http://"):
		return "ws://" + strings.TrimPrefix(value, "http://")
	case strings.HasPrefix(value, "https://"):
		return "wss://" + strings.TrimPrefix(value, "https://")
	case strings.HasPrefix(value, "ws://"), strings.HasPrefix(value, "wss://"):
		return value
	default:
		return "ws://" + value
	}
}

func main() {
	var cfg simulatorConfig
	flag.StringVar(&cfg.serverURL, "url", envOr("IOT_SIMULATOR_WS_URL", "ws://localhost:8080/api/v1/ws"), "Backend WebSocket URL")
	flag.StringVar(&cfg.token, "token", os.Getenv("IOT_SIMULATOR_TOKEN"), "access token; empty token requires ALLOW_ANONYMOUS=true")
	flag.StringVar(&cfg.roomID, "room-id", envOr("IOT_SIMULATOR_ROOM_ID", "room-1"), "room ID")
	flag.StringVar(&cfg.userID, "user-id", envOr("IOT_SIMULATOR_USER_ID", "user-1"), "user ID for anonymous mode")
	flag.StringVar(&cfg.clientID, "client-id", envOr("IOT_SIMULATOR_CLIENT_ID", "iot-simulator-01"), "simulated device ID")
	flag.Parse()

	cfg.serverURL = websocketURL(cfg.serverURL)
	if err := newSimulator(cfg).run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
