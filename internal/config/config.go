package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	CloudURL         string `yaml:"cloud_url"`
	PairCode         string `yaml:"pair_code"`
	AgentID          uint64 `yaml:"agent_id"`
	AgentSecret      string `yaml:"agent_secret"`
	HAURL            string `yaml:"ha_url"`
	HAToken          string `yaml:"ha_token"`
	StateDir         string `yaml:"state_dir"`
	HeartbeatSeconds int    `yaml:"heartbeat_seconds"`
	ReconnectMin     int    `yaml:"reconnect_min_seconds"`
	ReconnectMax     int    `yaml:"reconnect_max_seconds"`
}

type State struct {
	AgentID     uint64 `yaml:"agent_id"`
	AgentSecret string `yaml:"agent_secret"`
}

func Default() *Config {
	return &Config{
		CloudURL:         "wss://elianiot.com/api/v1/agent/connect",
		HAURL:            "http://127.0.0.1:8123",
		StateDir:         "./data",
		HeartbeatSeconds: 30,
		ReconnectMin:     2,
		ReconnectMax:     60,
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("读取配置失败: %w", err)
			}
		} else {
			if err := yaml.Unmarshal(raw, cfg); err != nil {
				return nil, fmt.Errorf("解析配置失败: %w", err)
			}
		}
	}
	applyEnv(cfg)
	if cfg.StateDir == "" {
		cfg.StateDir = "./data"
	}
	if cfg.HeartbeatSeconds <= 0 {
		cfg.HeartbeatSeconds = 30
	}
	if cfg.ReconnectMin <= 0 {
		cfg.ReconnectMin = 2
	}
	if cfg.ReconnectMax < cfg.ReconnectMin {
		cfg.ReconnectMax = 60
	}
	if err := cfg.LoadState(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if err := validateWebSocketURL(c.CloudURL); err != nil {
		return fmt.Errorf("cloud_url 无效: %w", err)
	}
	if c.HAURL == "" {
		return errors.New("ha_url 不能为空")
	}
	if _, err := url.ParseRequestURI(c.HAURL); err != nil {
		return fmt.Errorf("ha_url 无效: %w", err)
	}
	if c.HAToken == "" {
		return errors.New("ha_token 不能为空")
	}
	if c.AgentID == 0 && strings.TrimSpace(c.PairCode) == "" {
		return errors.New("首次启动必须提供 pair_code，或提供已保存的 agent_id/agent_secret")
	}
	if (c.AgentID == 0) != (c.AgentSecret == "") {
		return errors.New("agent_id 和 agent_secret 必须同时提供")
	}
	return nil
}

func validateWebSocketURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return errors.New("必须是 ws 或 wss 地址")
	}
	if u.Host == "" {
		return errors.New("缺少主机名")
	}
	return nil
}

func applyEnv(c *Config) {
	applyString("ELIAN_CLOUD_URL", &c.CloudURL)
	applyString("ELIAN_PAIR_CODE", &c.PairCode)
	applyString("ELIAN_AGENT_SECRET", &c.AgentSecret)
	applyString("ELIAN_HA_URL", &c.HAURL)
	applyString("ELIAN_HA_TOKEN", &c.HAToken)
	applyString("ELIAN_STATE_DIR", &c.StateDir)
	applyUint64("ELIAN_AGENT_ID", &c.AgentID)
}

func applyString(key string, target *string) {
	if value := os.Getenv(key); value != "" {
		*target = value
	}
}

func applyUint64(key string, target *uint64) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			*target = parsed
		}
	}
}

func (c *Config) StatePath() string {
	return filepath.Join(c.StateDir, "agent-state.yaml")
}

func (c *Config) LoadState() error {
	raw, err := os.ReadFile(c.StatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取 Agent 状态失败: %w", err)
	}
	var state State
	if err := yaml.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("解析 Agent 状态失败: %w", err)
	}
	if state.AgentID > 0 {
		c.AgentID = state.AgentID
	}
	if state.AgentSecret != "" {
		c.AgentSecret = state.AgentSecret
	}
	return nil
}

func (c *Config) SaveState() error {
	if err := os.MkdirAll(c.StateDir, 0o700); err != nil {
		return fmt.Errorf("创建状态目录失败: %w", err)
	}
	raw, err := yaml.Marshal(State{
		AgentID:     c.AgentID,
		AgentSecret: c.AgentSecret,
	})
	if err != nil {
		return err
	}
	tmp := c.StatePath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("写入 Agent 状态失败: %w", err)
	}
	if err := os.Rename(tmp, c.StatePath()); err != nil {
		return fmt.Errorf("保存 Agent 状态失败: %w", err)
	}
	return nil
}
