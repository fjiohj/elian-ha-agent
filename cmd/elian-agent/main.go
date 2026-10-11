package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fjiohj/elian-ha-agent/internal/cloud"
	"github.com/fjiohj/elian-ha-agent/internal/config"
	"github.com/fjiohj/elian-ha-agent/internal/ha"
	"github.com/fjiohj/elian-ha-agent/internal/version"
)

func main() {
	defaultConfig := os.Getenv("ELIAN_CONFIG")
	if defaultConfig == "" {
		defaultConfig = "agent.yaml"
	}
	var (
		configPath = flag.String("config", defaultConfig, "Agent 配置文件")
		showVer    = flag.Bool("version", false, "打印版本")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("elian-ha-agent %s (%s, %s)\n", version.Version, version.Commit, version.Date)
		return
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	haClient, err := ha.NewClient(cfg.HAURL, cfg.HAToken)
	if err != nil {
		log.Fatalf("创建 HA 客户端失败: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("启动 elian-ha-agent %s，云端: %s", version.Version, cfg.CloudURL)
	if err := cloud.NewClient(cfg, haClient).Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("Agent 退出: %v", err)
	}
}
