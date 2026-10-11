package protocol

import (
	"encoding/json"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	input := Message{
		Type:   TypeResponse,
		ID:     "req-1",
		Status: 200,
		Data:   json.RawMessage(`{"ok":true}`),
	}
	raw, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if output.Type != input.Type || output.ID != input.ID || output.Status != input.Status {
		t.Fatalf("消息往返失败: %+v", output)
	}
}
