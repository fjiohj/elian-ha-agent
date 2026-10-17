# 部署

## Home Assistant Add-on

1. 在 HA 中添加 Add-on 仓库：

```text
https://github.com/fjiohj/elian-ha-agent
```

2. 安装 `elian HA Agent`
3. 启动 Add-on，无需填写配置
4. 打开 Add-on 日志中的授权链接
5. 登录 elianiot 并确认授权

如果更换了 elianiot 账号，需要先删除 Add-on 数据目录中的 `agent-state.yaml`。重启后会显示新的授权链接，否则 Agent 会继续使用旧身份自动登录。

Add-on 优先使用 `http://supervisor/core` 和 `SUPERVISOR_TOKEN`。

## Docker

```bash
mkdir -p /opt/elian-agent

cat > /opt/elian-agent/.env <<'EOF'
ELIAN_CLOUD_URL=wss://elianiot.com/api/v1/agent/connect
ELIAN_HA_URL=http://127.0.0.1:8123
ELIAN_HA_TOKEN=YOUR_LONG_LIVED_TOKEN
ELIAN_STATE_DIR=/data
EOF

chmod 600 /opt/elian-agent/.env

docker run -d \
  --name elian-ha-agent \
  --restart unless-stopped \
  --network host \
  --env-file /opt/elian-agent/.env \
  -v elian-agent-data:/data \
  ghcr.io/fjiohj/elian-ha-agent:latest
```

首次启动时日志会显示授权链接和短码。登录 elianiot 确认后，长期身份保存在 `/data/agent-state.yaml`，容器重建时请保留 `/data` 卷。

## systemd

```ini
[Unit]
Description=elian HA Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=elian
WorkingDirectory=/opt/elian-agent
ExecStart=/opt/elian-agent/elian-agent --config /opt/elian-agent/agent.yaml
Restart=always
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```
