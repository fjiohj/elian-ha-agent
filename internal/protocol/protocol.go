package protocol

import "encoding/json"

const Version = 1

const (
	TypePair     = "pair"
	TypePaired   = "paired"
	TypeAuth     = "auth"
	TypeAuthOK   = "auth_ok"
	TypeRequest  = "request"
	TypeResponse = "response"
	TypeEvent    = "event"
	TypePing     = "ping"
	TypePong     = "pong"
	TypeError    = "error"
)

// Message 是云端与 Agent 之间统一的双向消息。
// Body/Data 使用 json.RawMessage，避免 Agent 理解云端业务对象。
type Message struct {
	Type string `json:"type"`

	Version int    `json:"version,omitempty"`
	ID      string `json:"id,omitempty"`
	AgentID uint64 `json:"agent_id,omitempty"`

	// PairCode 只用于首次配对。
	PairCode string `json:"pair_code,omitempty"`
	// AgentSecret 用于持久身份认证。云端只保存 hash。
	AgentSecret string `json:"agent_secret,omitempty"`

	AgentVersion string `json:"agent_version,omitempty"`
	HAVersion    string `json:"ha_version,omitempty"`
	Platform     string `json:"platform,omitempty"`

	// Request 字段。
	Method string          `json:"method,omitempty"`
	Path   string          `json:"path,omitempty"`
	Body   json.RawMessage `json:"body,omitempty"`

	// Response 字段。
	Status int             `json:"status,omitempty"`
	Error  string          `json:"error,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`

	// Event 字段。
	Event string          `json:"event,omitempty"`
	Time  int64           `json:"time,omitempty"`
	Extra json.RawMessage `json:"extra,omitempty"`
}

func Encode(message Message) ([]byte, error) {
	return json.Marshal(message)
}

func Decode(data []byte) (Message, error) {
	var message Message
	if err := json.Unmarshal(data, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}
