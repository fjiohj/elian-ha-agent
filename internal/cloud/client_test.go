package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fjiohj/elian-ha-agent/internal/config"
	"github.com/fjiohj/elian-ha-agent/internal/ha"
	"github.com/fjiohj/elian-ha-agent/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestClientPairsAndProxiesRequest(t *testing.T) {
	haServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/states" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"entity_id":"light.test"}]`))
	}))
	defer haServer.Close()

	responseCh := make(chan protocol.Message, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("读取配对消息失败: %v", err)
			return
		}
		message, err := protocol.Decode(raw)
		if err != nil || message.Type != protocol.TypePair || message.PairCode != "PAIR" {
			t.Errorf("配对消息错误: %s %v", raw, err)
			return
		}
		if err := conn.WriteJSON(protocol.Message{
			Type:        protocol.TypePaired,
			AgentID:     99,
			AgentSecret: "long-secret",
		}); err != nil {
			return
		}
		if err := conn.WriteJSON(protocol.Message{
			Type:   protocol.TypeRequest,
			ID:     "req-1",
			Method: http.MethodGet,
			Path:   "/api/states",
		}); err != nil {
			return
		}
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			message, err := protocol.Decode(raw)
			if err == nil && message.Type == protocol.TypeResponse && message.ID == "req-1" {
				responseCh <- message
				return
			}
		}
	}))
	defer cloudServer.Close()

	haClient, err := ha.NewClient(haServer.URL, "ha-token")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		CloudURL:         "ws" + strings.TrimPrefix(cloudServer.URL, "http"),
		PairCode:         "PAIR",
		HAURL:            haServer.URL,
		HAToken:          "ha-token",
		StateDir:         t.TempDir(),
		HeartbeatSeconds: 30,
		ReconnectMin:     1,
		ReconnectMax:     2,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewClient(cfg, haClient).Run(ctx)
	}()

	select {
	case response := <-responseCh:
		if response.Status != http.StatusOK {
			t.Fatalf("代理响应状态错误: %+v", response)
		}
		var data []map[string]string
		if err := json.Unmarshal(response.Data, &data); err != nil {
			t.Fatalf("代理响应 data 错误: %v", err)
		}
		if len(data) != 1 || data[0]["entity_id"] != "light.test" {
			t.Fatalf("代理响应 data 错误: %+v", data)
		}
		if cfg.AgentID != 99 || cfg.AgentSecret != "long-secret" {
			t.Fatalf("配对状态未保存: %+v", cfg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("等待代理响应超时")
	}
}
