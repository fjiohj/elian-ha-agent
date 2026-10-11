# 部署

## Home Assistant Add-on

1. 在 HA 中添加 Add-on 仓库：

```text
https://github.com/fjiohj/elian-ha-agent
```

2. 安装 `elian HA Agent`
3. 在 elianiot 平台生成一次性绑定码
4. 在 Add-on 配置中填写 `pair_code`
5. 启动 Add-on

Add-on 优先使用 `http://supervisor/core` 和 `SUPERVISOR_TOKEN`。

## Docker

```bash
mkdir -p /opt/elian-agent

cat > /opt/elian-agent/.env <<'EOF'
ELIAN_CLOUD_URL=wss://elianiot.com/api/v1/agent/connect
ELIAN_PAIR_CODE=AGENT-XXXX-XXXX
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
