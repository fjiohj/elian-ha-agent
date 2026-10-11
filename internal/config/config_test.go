package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.StateDir = dir
	cfg.AgentID = 42
	cfg.AgentSecret = "secret"
	if err := cfg.SaveState(); err != nil {
		t.Fatal(err)
	}

	loaded := Default()
	loaded.StateDir = dir
	loaded.HAToken = "token"
	loaded.PairCode = "unused"
	if err := loaded.LoadState(); err != nil {
		t.Fatal(err)
	}
	if loaded.AgentID != 42 || loaded.AgentSecret != "secret" {
		t.Fatalf("状态未恢复: %+v", loaded)
	}
	info, err := os.Stat(filepath.Join(dir, "agent-state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("状态文件权限应为 0600，实际 %o", info.Mode().Perm())
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	t.Setenv("ELIAN_CLOUD_URL", "wss://cloud.example/agent")
	t.Setenv("ELIAN_HA_URL", "http://ha.example:8123")
	t.Setenv("ELIAN_HA_TOKEN", "token")
	t.Setenv("ELIAN_PAIR_CODE", "PAIR")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CloudURL != "wss://cloud.example/agent" || cfg.HAURL != "http://ha.example:8123" {
		t.Fatalf("环境变量未生效: %+v", cfg)
	}
}

func TestEnvironmentOnlyConfigWithoutFile(t *testing.T) {
	t.Setenv("ELIAN_CLOUD_URL", "wss://cloud.example/agent")
	t.Setenv("ELIAN_HA_URL", "http://ha.example:8123")
	t.Setenv("ELIAN_HA_TOKEN", "token")
	t.Setenv("ELIAN_PAIR_CODE", "PAIR")
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PairCode != "PAIR" {
		t.Fatalf("环境变量模式失败: %+v", cfg)
	}
}
