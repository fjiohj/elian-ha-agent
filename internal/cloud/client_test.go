package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestClientHandlesWSCommand(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	haServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/websocket" {
			http.NotFound(w, r)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(map[string]string{"type": "auth_required"})
		var auth map[string]string
		_ = conn.ReadJSON(&auth)
		_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})
		for {
			var command map[string]interface{}
			if err := conn.ReadJSON(&command); err != nil {
				return
			}
			if command["type"] == "subscribe_events" {
				_ = conn.WriteJSON(map[string]interface{}{
					"id": command["id"], "type": "result", "success": true,
				})
				continue
			}
			if command["type"] != "config/area_registry/list" {
				t.Errorf("HA 命令错误: %+v", command)
				return
			}
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      command["id"],
				"type":    "result",
				"success": true,
				"result":  []map[string]string{{"area_id": "living", "name": "客厅"}},
			})
			return
		}
	}))
	defer haServer.Close()

	resultCh := make(chan protocol.Message, 1)
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		_ = conn.WriteJSON(protocol.Message{
			Type:        protocol.TypePaired,
			AgentID:     100,
			AgentSecret: "long-secret",
		})
		if err := conn.WriteJSON(protocol.Message{
			Type:    protocol.TypeWSCommand,
			ID:      "ws-1",
			Command: "config/area_registry/list",
		}); err != nil {
			return
		}
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			message, err := protocol.Decode(raw)
			if err == nil && message.Type == protocol.TypeWSResult && message.ID == "ws-1" {
				resultCh <- message
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
	case result := <-resultCh:
		if result.Error != "" {
			t.Fatalf("WS 命令返回错误: %s", result.Error)
		}
		if !strings.Contains(string(result.Data), `"name":"客厅"`) {
			t.Fatalf("WS 命令结果错误: %s", result.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("等待 WS 命令结果超时")
	}
}

func TestClientDeviceAuthorization(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agent/device-code":
			_, _ = w.Write([]byte(`{
				"code":0,
				"data":{
					"device_code":"device-secret",
					"user_code":"ABCD-EFGH",
					"interval":1,
					"verification_uri":"http://example/activate",
					"verification_uri_complete":"http://example/activate?code=ABCD-EFGH"
				}
			}`))
		case "/api/v1/agent/device-token":
			pollCount++
			if pollCount == 1 {
				_, _ = w.Write([]byte(`{"code":0,"data":{"status":"pending"}}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"code":0,
				"data":{"status":"authorized","agent_id":88,"agent_secret":"long-secret"}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		CloudURL: "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/agent/connect",
		HAURL:    "http://127.0.0.1:8123",
		HAToken:  "ha-token",
		StateDir: t.TempDir(),
	}
	haClient, err := ha.NewClient(cfg.HAURL, cfg.HAToken)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(cfg, haClient)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.authorizeDevice(ctx); err != nil {
		t.Fatal(err)
	}
	if cfg.AgentID != 88 || cfg.AgentSecret != "long-secret" {
		t.Fatalf("设备授权身份未保存: %+v", cfg)
	}
	if pollCount < 2 {
		t.Fatalf("应至少轮询两次，实际 %d", pollCount)
	}
	state, err := os.ReadFile(cfg.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), "agent_id: 88") {
		t.Fatalf("身份状态未落盘: %s", state)
	}
}
