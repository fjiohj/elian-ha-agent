package ha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestEventStreamParsesHAStateChangedEnvelope(t *testing.T) {
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
		var subscribe map[string]interface{}
		_ = conn.ReadJSON(&subscribe)
		_ = conn.WriteJSON(map[string]interface{}{
			"id": subscribe["id"], "type": "result", "success": true,
		})
		_ = conn.WriteJSON(map[string]interface{}{
			"id":   subscribe["id"],
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "state_changed",
				"data": map[string]interface{}{
					"entity_id": "light.test",
					"new_state": map[string]interface{}{
						"entity_id": "light.test",
						"state":     "on",
					},
				},
			},
		})
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	eventCh := make(chan json.RawMessage, 1)
	stream, err := NewEventStream(server.URL, "ha-token", func(event string, data json.RawMessage) {
		if event == "state_changed" {
			eventCh <- data
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = stream.Run(ctx) }()

	select {
	case data := <-eventCh:
		if !strings.Contains(string(data), `"entity_id":"light.test"`) {
			t.Fatalf("事件数据错误: %s", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("未收到 state_changed 事件")
	}
}
