package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/fjiohj/elian-ha-agent/internal/config"
	"github.com/fjiohj/elian-ha-agent/internal/ha"
	"github.com/fjiohj/elian-ha-agent/internal/protocol"
	"github.com/fjiohj/elian-ha-agent/internal/version"
	"github.com/gorilla/websocket"
)

type Client struct {
	cfg *config.Config
	ha  *ha.Client

	writeMu sync.Mutex
	connMu  sync.RWMutex
	conn    *websocket.Conn
}

func NewClient(cfg *config.Config, haClient *ha.Client) *Client {
	return &Client{cfg: cfg, ha: haClient}
}

func (c *Client) Run(ctx context.Context) error {
	backoff := time.Duration(c.cfg.ReconnectMin) * time.Second
	maxBackoff := time.Duration(c.cfg.ReconnectMax) * time.Second
	for {
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			log.Printf("云端连接断开: %v", err)
		}
		jitter := time.Duration(rand.Int63n(int64(backoff/2 + 1)))
		wait := backoff + jitter
		log.Printf("%s 后重连云端", wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	header := map[string][]string{
		"X-Agent-Version": {version.Version},
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.cfg.CloudURL, header)
	if err != nil {
		return err
	}
	conn.SetReadLimit(16 << 20)
	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()
	defer func() {
		c.connMu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.connMu.Unlock()
		_ = conn.Close()
	}()

	if c.cfg.AgentID > 0 && c.cfg.AgentSecret != "" {
		if err := c.write(conn, protocol.Message{
			Type:         protocol.TypeAuth,
			Version:      protocol.Version,
			AgentID:      c.cfg.AgentID,
			AgentSecret:  c.cfg.AgentSecret,
			AgentVersion: version.Version,
		}); err != nil {
			return err
		}
	} else {
		if err := c.write(conn, protocol.Message{
			Type:         protocol.TypePair,
			Version:      protocol.Version,
			PairCode:     c.cfg.PairCode,
			AgentVersion: version.Version,
			Platform:     "homeassistant",
		}); err != nil {
			return err
		}
	}

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	var readyOnce sync.Once
	startBackground := func() {
		readyOnce.Do(func() {
			go c.runHAEventStream(streamCtx)
			go c.runHeartbeat(streamCtx, conn)
		})
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		message, err := protocol.Decode(raw)
		if err != nil {
			log.Printf("忽略无法解析的云端消息: %v", err)
			continue
		}

		switch message.Type {
		case protocol.TypePaired:
			if message.AgentID == 0 || message.AgentSecret == "" {
				return errors.New("云端配对响应缺少 agent_id 或 agent_secret")
			}
			c.cfg.AgentID = message.AgentID
			c.cfg.AgentSecret = message.AgentSecret
			c.cfg.PairCode = ""
			if err := c.cfg.SaveState(); err != nil {
				return err
			}
			log.Printf("Agent 配对成功，agent_id=%d", c.cfg.AgentID)
			startBackground()
		case protocol.TypeAuthOK:
			log.Printf("Agent 认证成功，agent_id=%d", c.cfg.AgentID)
			startBackground()
		case protocol.TypeRequest:
			go c.handleRequest(streamCtx, conn, message)
		case protocol.TypePing:
			_ = c.write(conn, protocol.Message{
				Type: protocol.TypePong,
				ID:   message.ID,
				Time: time.Now().Unix(),
			})
		case protocol.TypePong:
			// 心跳响应，无需处理。
		case protocol.TypeError:
			log.Printf("云端返回错误: %s", message.Error)
		}
	}
}

func (c *Client) handleRequest(ctx context.Context, conn *websocket.Conn, message protocol.Message) {
	if message.ID == "" {
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	status, body, err := c.ha.Do(reqCtx, message.Method, message.Path, message.Body)
	response := protocol.Message{
		Type:   protocol.TypeResponse,
		ID:     message.ID,
		Status: status,
	}
	if err != nil {
		response.Status = 502
		response.Error = err.Error()
		response.Data = json.RawMessage(`null`)
	} else if json.Valid(body) {
		response.Data = json.RawMessage(body)
	} else {
		wrapped, _ := json.Marshal(map[string]string{"raw": string(body)})
		response.Data = wrapped
	}
	if err := c.write(conn, response); err != nil {
		log.Printf("发送 HA 代理响应失败: %v", err)
	}
}

func (c *Client) runHAEventStream(ctx context.Context) {
	stream, err := ha.NewEventStream(c.cfg.HAURL, c.cfg.HAToken, func(event string, data json.RawMessage) {
		conn := c.currentConn()
		if conn == nil {
			return
		}
		if err := c.write(conn, protocol.Message{
			Type:  protocol.TypeEvent,
			Event: event,
			Time:  time.Now().Unix(),
			Data:  data,
		}); err != nil {
			log.Printf("推送 HA 事件失败: %v", err)
		}
	})
	if err != nil {
		log.Printf("创建 HA WebSocket 客户端失败: %v", err)
		return
	}
	if err := stream.Run(ctx); err != nil && ctx.Err() == nil {
		log.Printf("HA WebSocket 事件流退出: %v", err)
	}
}

func (c *Client) runHeartbeat(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(time.Duration(c.cfg.HeartbeatSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := c.write(conn, protocol.Message{
				Type: protocol.TypePing,
				Time: now.Unix(),
			}); err != nil {
				return
			}
		}
	}
}

func (c *Client) currentConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}

func (c *Client) write(conn *websocket.Conn, message protocol.Message) error {
	raw, err := protocol.Encode(message)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, raw)
}
