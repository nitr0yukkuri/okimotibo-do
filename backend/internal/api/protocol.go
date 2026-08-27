package api

import (
	"encoding/json"
	"time"

	"github.com/NxTEND-THE-HACK/2026-Team-02/backend/internal/domain"
)

type envelope struct {
	Type string `json:"type"`
}

type helloMessage struct {
	Type     string `json:"type"`
	Token    string `json:"token"`
	RoomID   string `json:"roomId"`
	ClientID string `json:"clientId"`
	UserID   string `json:"userId,omitempty"`
}

type recognitionMessage struct {
	Type       string        `json:"type"`
	Sequence   uint64        `json:"sequence"`
	CapturedAt time.Time     `json:"capturedAt"`
	Status     domain.Status `json:"status"`
	Hand       *domain.Hand  `json:"hand"`
	Face       *domain.Face  `json:"face"`
	Source     domain.Source `json:"source"`
}

type outgoingMessage struct {
	Type       string        `json:"type"`
	State      *domain.State `json:"state,omitempty"`
	RoomID     string        `json:"roomId,omitempty"`
	UserID     string        `json:"userId,omitempty"`
	ClientID   string        `json:"clientId,omitempty"`
	CapturedAt time.Time     `json:"capturedAt,omitempty"`
	ReceivedAt time.Time     `json:"receivedAt,omitempty"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
}

func messageType(data []byte) (string, error) {
	var value envelope
	err := json.Unmarshal(data, &value)
	return value.Type, err
}
