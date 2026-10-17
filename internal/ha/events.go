package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type EventHandler func(event string, data json.RawMessage)

type EventStream struct {
	baseURL *url.URL
	token   string
	handler EventHandler
}

func NewEventStream(rawBaseURL, token string, handler EventHandler) (*EventStream, error) {
	baseURL, err := url.Parse(strings.TrimRight(rawBaseURL, "/"))
	if err != nil {
		return nil, err
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errors.New("HA 地址必须是 http 或 https")
	}
	return &EventStream{baseURL: baseURL, token: token, handler: handler}, nil
}

func (s *EventStream) Run(ctx context.Context) error {
	backoff := 2 * time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.runOnce(ctx); err != nil {
			log.Printf("HA WebSocket 连接断开: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (s *EventStream) runOnce(ctx context.Context) error {
	wsURL := *s.baseURL
	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = strings.TrimRight(s.baseURL.Path, "/") + "/api/websocket"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	var first map[string]interface{}
	if err := conn.ReadJSON(&first); err != nil {
		return err
	}
	if first["type"] != "auth_required" {
		return fmt.Errorf("HA WebSocket 未请求认证: %v", first["type"])
	}
	if err := conn.WriteJSON(map[string]string{
		"type":         "auth",
		"access_token": s.token,
	}); err != nil {
		return err
	}

	var authReply map[string]interface{}
	if err := conn.ReadJSON(&authReply); err != nil {
		return err
	}
	if authReply["type"] != "auth_ok" {
		return fmt.Errorf("HA WebSocket 认证失败: %v", authReply["message"])
	}
	if err := conn.WriteJSON(map[string]interface{}{
		"id":         1,
		"type":       "subscribe_events",
		"event_type": "state_changed",
	}); err != nil {
		return err
	}
	for {
		var ack struct {
			Type    string `json:"type"`
			ID      int    `json:"id"`
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		if err := conn.ReadJSON(&ack); err != nil {
			return err
		}
		if ack.Type == "result" && ack.ID == 1 {
			if !ack.Success {
				return fmt.Errorf("HA 订阅 state_changed 失败: %s", ack.Message)
			}
			break
		}
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var message struct {
			Type  string `json:"type"`
			Event struct {
				EventType string          `json:"event_type"`
				Data      json.RawMessage `json:"data"`
			} `json:"event"`
		}
		if json.Unmarshal(raw, &message) != nil {
			continue
		}
		if message.Type == "event" && message.Event.EventType == "state_changed" {
			s.handler(message.Event.EventType, message.Event.Data)
		}
	}
}
