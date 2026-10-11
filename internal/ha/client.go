package ha

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 16 << 20

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
