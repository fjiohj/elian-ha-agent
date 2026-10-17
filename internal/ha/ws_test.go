package ha

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWSCommandAuthenticatesAndReturnsRegistry(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if err := conn.ReadJSON(&auth); err != nil {
			t.Errorf("读取认证消息失败: %v", err)
			return
		}
		if auth["type"] != "auth" || auth["access_token"] != "ha-token" {
			t.Errorf("认证消息错误: %+v", auth)
			return
		}
		_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})

		var command map[string]interface{}
		if err := conn.ReadJSON(&command); err != nil {
			t.Errorf("读取命令失败: %v", err)
			return
		}
		if command["type"] != "config/device_registry/list" || command["id"] != float64(1) {
			t.Errorf("命令错误: %+v", command)
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"id":      1,
			"type":    "result",
			"success": true,
			"result":  []map[string]string{{"id": "dev-1", "name": "门磁"}},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "ha-token")
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.WSCommand(context.Background(), "config/device_registry/list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"name":"门磁"`) {
		t.Fatalf("注册表结果错误: %s", data)
	}
}

func TestWSCommandRejectsUnlistedCommand(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:8123", "ha-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WSCommand(context.Background(), "config/device_registry/remove"); err == nil {
		t.Fatal("白名单外命令应被拒绝")
	}
}

func TestWSCommandReturnsHAFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(map[string]string{"type": "auth_required"})
		var auth map[string]string
		_ = conn.ReadJSON(&auth)
		_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})
		var command map[string]interface{}
		_ = conn.ReadJSON(&command)
		_ = conn.WriteJSON(map[string]interface{}{
			"id": 1, "type": "result", "success": false, "message": "权限不足",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "ha-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WSCommand(context.Background(), "config/entity_registry/list"); err == nil ||
		!strings.Contains(err.Error(), "权限不足") {
		t.Fatalf("HA 错误未正确返回: %v", err)
	}
}
