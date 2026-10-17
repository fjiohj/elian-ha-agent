package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
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
		if c.cfg.AgentID == 0 && strings.TrimSpace(c.cfg.PairCode) == "" {
			if err := c.authorizeDevice(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("设备授权失败: %v", err)
				jitter := time.Duration(rand.Int63n(int64(backoff/2 + 1)))
				wait := backoff + jitter
				log.Printf("%s 后重试设备授权", wait.Round(time.Second))
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
			backoff = time.Duration(c.cfg.ReconnectMin) * time.Second
		}

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

type deviceCodeData struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	Interval                int    `json:"interval"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
}

type deviceTokenData struct {
	Status      string `json:"status"`
	AgentID     uint64 `json:"agent_id"`
	AgentSecret string `json:"agent_secret"`
}

type cloudEnvelope struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// authorizeDevice 实现 OAuth Device Authorization：
// Agent 无配置启动 → 展示短码和授权链接 → 用户登录平台确认 → Agent 自动取得长期身份。
func (c *Client) authorizeDevice(ctx context.Context) error {
	var start deviceCodeData
	if err := c.postJSON(ctx, "device-code", map[string]string{
		"agent_version": version.Version,
	}, &start); err != nil {
		return err
	}
	if start.DeviceCode == "" || start.UserCode == "" {
		return errors.New("云端未返回设备授权信息")
	}
	interval := time.Duration(start.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	link := start.VerificationURIComplete
	if link == "" {
		link = start.VerificationURI
	}
	log.Printf("请在浏览器打开: %s", link)
	log.Printf("如果页面要求输入授权码，请输入: %s", start.UserCode)
	log.Printf("等待用户在 elianiot 平台确认授权...")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var token deviceTokenData
			if err := c.postJSON(ctx, "device-token", map[string]string{
				"device_code": start.DeviceCode,
			}, &token); err != nil {
				return err
			}
			switch token.Status {
			case "pending":
				continue
			case "authorized":
				if token.AgentID == 0 || token.AgentSecret == "" {
					return errors.New("设备授权结果缺少 agent_id 或 agent_secret")
				}
				if err := c.cfg.SaveIdentity(token.AgentID, token.AgentSecret); err != nil {
					return err
				}
				c.cfg.AgentID = token.AgentID
				c.cfg.AgentSecret = token.AgentSecret
				c.cfg.PairCode = ""
				log.Printf("Agent 设备授权成功，agent_id=%d", c.cfg.AgentID)
				return nil
			default:
				return errors.New("云端返回了未知设备授权状态")
			}
		}
	}
}

func (c *Client) postJSON(ctx context.Context, action string, payload, target interface{}) error {
	endpoint, err := c.deviceEndpoint(action)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope cloudEnvelope
	_ = json.Unmarshal(body, &envelope)
	if resp.StatusCode >= http.StatusBadRequest ||
		strings.TrimSpace(string(envelope.Code)) != "0" {
		if envelope.Message != "" {
			return errors.New(envelope.Message)
		}
		return errors.New("云端设备授权接口返回错误")
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errors.New("云端设备授权响应缺少数据")
	}
	return json.Unmarshal(envelope.Data, target)
}

func (c *Client) deviceEndpoint(action string) (string, error) {
	u, err := url.Parse(c.cfg.CloudURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "wss" {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}
	u.Path = strings.TrimSuffix(u.Path, "/connect") + "/" + action
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
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
			if err := c.cfg.SaveIdentity(message.AgentID, message.AgentSecret); err != nil {
				return err
			}
			c.cfg.AgentID = message.AgentID
			c.cfg.AgentSecret = message.AgentSecret
			c.cfg.PairCode = ""
			log.Printf("Agent 配对成功，agent_id=%d", c.cfg.AgentID)
			startBackground()
		case protocol.TypeAuthOK:
			log.Printf("Agent 认证成功，agent_id=%d", c.cfg.AgentID)
			startBackground()
		case protocol.TypeRequest:
			go c.handleRequest(streamCtx, conn, message)
		case protocol.TypeWSCommand:
			go c.handleWSCommand(streamCtx, conn, message)
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

func (c *Client) handleWSCommand(ctx context.Context, conn *websocket.Conn, message protocol.Message) {
	if message.ID == "" || message.Command == "" {
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	data, err := c.ha.WSCommand(reqCtx, message.Command)
	response := protocol.Message{
		Type: protocol.TypeWSResult,
		ID:   message.ID,
	}
	if err != nil {
		response.Error = err.Error()
	} else {
		response.Data = data
	}
	if err := c.write(conn, response); err != nil {
		log.Printf("发送 HA WebSocket 命令结果失败: %v", err)
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
