# elian HA Agent

将本地 Home Assistant 安全接入 `elianiot.com` 的专用边缘代理。

Agent 只主动连接云端，不开放入站端口：

```text
Home Assistant
      ↕
elian HA Agent
      ↕ WSS
elianiot.com
```

## 能力

- 一次性绑定码配对
- Agent 长期身份持久化
- HA REST API 白名单代理
- HA WebSocket `state_changed` 事件推送
- 心跳与断线指数退避重连
- Docker / Home Assistant Add-on 部署

详细设计和部署见 `docs/` 与根目录的 `elian_agent/`。

## 快速开始

### Home Assistant Add-on

在 HA 中添加仓库：

```text
https://github.com/fjiohj/elian-ha-agent
```

安装 `elian HA Agent`，填写平台生成的一次性 `pair_code` 后启动。

### Docker

```bash
docker run -d \
  --name elian-ha-agent \
  --restart unless-stopped \
  --network host \
  -e ELIAN_CLOUD_URL=wss://elianiot.com/api/v1/agent/connect \
  -e ELIAN_PAIR_CODE=AGENT-XXXX-XXXX \
  -e ELIAN_HA_URL=http://127.0.0.1:8123 \
  -e ELIAN_HA_TOKEN=YOUR_LONG_LIVED_TOKEN \
  -e ELIAN_STATE_DIR=/data \
  -v elian-agent-data:/data \
  ghcr.io/fjiohj/elian-ha-agent:latest
```

### 本地运行

```bash
cp agent.example.yaml agent.yaml
# 填写 pair_code、ha_token
go run ./cmd/elian-agent --config agent.yaml
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `ELIAN_CLOUD_URL` | Agent Hub WebSocket 地址 |
| `ELIAN_PAIR_CODE` | 首次绑定码 |
| `ELIAN_AGENT_ID` | 配对后 Agent ID |
| `ELIAN_AGENT_SECRET` | 配对后长期密钥 |
| `ELIAN_HA_URL` | Home Assistant 地址 |
| `ELIAN_HA_TOKEN` | HA Long-Lived Access Token |
| `ELIAN_STATE_DIR` | Agent 状态目录 |

配对成功后，`agent_id` 和 `agent_secret` 保存到：

```text
state_dir/agent-state.yaml
```
