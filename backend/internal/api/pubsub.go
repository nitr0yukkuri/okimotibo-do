package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const statusEventsChannel = "okimochi:status-events"

type pubsubEnvelope struct {
	Origin  string          `json:"origin"`
	RoomID  string          `json:"roomId"`
	Message json.RawMessage `json:"message"`
}

type redisBus struct {
	client *redis.Client
	origin string
	logger *slog.Logger
}

func newRedisBus(rawURL, origin string, logger *slog.Logger) (*redisBus, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	options.DialTimeout = 5 * time.Second
	options.ReadTimeout = 0
	options.WriteTimeout = 5 * time.Second
	return &redisBus{client: redis.NewClient(options), origin: origin, logger: logger}, nil
}

func (b *redisBus) publish(ctx context.Context, roomID string, message []byte) error {
	payload, err := json.Marshal(pubsubEnvelope{Origin: b.origin, RoomID: roomID, Message: json.RawMessage(message)})
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, statusEventsChannel, payload).Err()
}

func (b *redisBus) run(ctx context.Context, deliver func(pubsubEnvelope)) {
	for {
		if ctx.Err() != nil {
			return
		}
		pubsub := b.client.Subscribe(ctx, statusEventsChannel)
		if _, err := pubsub.Receive(ctx); err != nil {
			_ = pubsub.Close()
			if !errors.Is(err, context.Canceled) {
				b.logger.Warn("redis subscribe failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		messages := pubsub.Channel()
		connected := true
		for connected {
			select {
			case <-ctx.Done():
				_ = pubsub.Close()
				return
			case message, ok := <-messages:
				if !ok {
					connected = false
					break
				}
				var event pubsubEnvelope
				if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
					b.logger.Warn("invalid redis status event", "error", err)
					continue
				}
				if event.Origin != b.origin && event.RoomID != "" && len(event.Message) > 0 {
					deliver(event)
				}
			}
		}
		_ = pubsub.Close()
		// A dropped subscription can otherwise make this loop spin while Redis
		// is unavailable. Keep reconnecting, but give the server a short
		// backoff so a broker outage does not turn into a CPU outage.
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (b *redisBus) close() error {
	return b.client.Close()
}
