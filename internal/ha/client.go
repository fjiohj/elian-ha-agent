package ha

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const maxResponseBytes = 16 << 20

// allowedWSCommands 是云端可以要求 Agent 执行的 HA 只读 WebSocket 命令。
// 不接受任意命令/参数，避免 Agent 成为本地 HA 的通用控制面。
var allowedWSCommands = map[string]struct{}{
	"config/entity_registry/list": {},
	"config/device_registry/list": {},
	"config/area_registry/list":   {},
}

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

func NewClient(rawBaseURL, token string) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(rawBaseURL, "/"))
	if err != nil {
		return nil, err
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errors.New("HA 地址必须是 http 或 https")
	}
	if baseURL.Host == "" {
		return nil, errors.New("HA 地址缺少主机名")
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (c *Client) Do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	relative, err := parseAPIPath(path)
	if err != nil {
		return 0, nil, err
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return 0, nil, fmt.Errorf("不支持的 HTTP 方法: %s", method)
	}

	target := *c.baseURL
	target.Path = strings.TrimRight(c.baseURL.Path, "/") + relative.Path
	target.RawQuery = relative.RawQuery

	var reader io.Reader
	if len(body) > 0 && string(body) != "null" {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, data, nil
}

// WSCommand 执行白名单内的 HA WebSocket 只读命令，供设备注册表同步使用。
func (c *Client) WSCommand(ctx context.Context, command string) (json.RawMessage, error) {
	if _, ok := allowedWSCommands[command]; !ok {
		return nil, fmt.Errorf("不允许的 HA WebSocket 命令: %s", command)
	}

	wsURL := *c.baseURL
	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = strings.TrimRight(c.baseURL.Path, "/") + "/api/websocket"

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, wsURL.String(), nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("连接 HA WebSocket 失败（HTTP %d）: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("连接 HA WebSocket 失败: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(maxResponseBytes)
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()

	deadline := time.Now().Add(30 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok {
		deadline = ctxDeadline
	}
	_ = conn.SetReadDeadline(deadline)

	var hello struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := conn.ReadJSON(&hello); err != nil {
		return nil, fmt.Errorf("读取 HA WebSocket 握手失败: %w", err)
	}
	if hello.Type != "auth_required" {
		return nil, fmt.Errorf("HA WebSocket 未要求认证: %s %s", hello.Type, hello.Message)
	}
	if err := conn.WriteJSON(map[string]string{
		"type":         "auth",
		"access_token": c.token,
	}); err != nil {
		return nil, fmt.Errorf("发送 HA WebSocket 认证失败: %w", err)
	}
	if err := conn.ReadJSON(&hello); err != nil {
		return nil, fmt.Errorf("读取 HA WebSocket 认证结果失败: %w", err)
	}
	if hello.Type != "auth_ok" {
		return nil, fmt.Errorf("HA WebSocket 认证失败: %s %s", hello.Type, hello.Message)
	}

	const commandID = 1
	if err := conn.WriteJSON(map[string]interface{}{
		"id":   commandID,
		"type": command,
	}); err != nil {
		return nil, fmt.Errorf("发送 HA WebSocket 命令失败: %w", err)
	}

	for {
		var message struct {
			Type    string          `json:"type"`
			ID      int             `json:"id"`
			Success bool            `json:"success"`
			Result  json.RawMessage `json:"result"`
			Message string          `json:"message"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			return nil, fmt.Errorf("读取 HA WebSocket 命令结果失败: %w", err)
		}
		if message.Type != "result" || message.ID != commandID {
			continue
		}
		if !message.Success {
			return nil, fmt.Errorf("HA 命令 %s 失败: %s", command, message.Message)
		}
		if !json.Valid(message.Result) {
			return nil, errors.New("HA WebSocket 命令返回了无效 JSON")
		}
		return message.Result, nil
	}
}

func parseAPIPath(path string) (*url.URL, error) {
	relative, err := url.ParseRequestURI(path)
	if err != nil {
		return nil, errors.New("非法 HA API 路径")
	}
	if relative.IsAbs() || relative.Host != "" {
		return nil, errors.New("不允许绝对 URL")
	}
	if err := validateAPIPath(relative.Path); err != nil {
		return nil, err
	}
	return relative, nil
}

func validateAPIPath(path string) error {
	if path == "" {
		return errors.New("路径不能为空")
	}
	if !strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "//") {
		return errors.New("只允许代理 HA /api/ 路径")
	}
	if strings.Contains(path, "..") || strings.Contains(path, "://") {
		return errors.New("非法 HA API 路径")
	}
	if strings.ContainsAny(path, "\r\n") {
		return errors.New("非法 HA API 路径")
	}
	return nil
}
