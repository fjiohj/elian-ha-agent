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

- 零配置设备授权（短码 + 浏览器确认）
- 兼容一次性绑定码配对
- Agent 长期身份持久化
- HA REST API 白名单代理
- HA WebSocket 注册表只读 RPC（实体/设备/区域）
- HA WebSocket `state_changed` 实时事件推送
- 心跳与断线指数退避重连
- Docker / Home Assistant Add-on 部署

详细设计和部署见 `docs/` 与根目录的 `elian_agent/`。

## 快速开始

### Home Assistant OS / Supervised（推荐）

1. 在 HA 打开「设置 → 加载项 → 加载项商店 → 右上角三个点 → 仓库」，添加：

   ```text
   https://github.com/fjiohj/elian-ha-agent
   ```

2. 安装 `elian HA Agent`。
3. 直接启动 Add-on，不需要填写任何配置。
4. 打开 Add-on「日志」，会看到：

   ```text
   请在浏览器打开: https://elianiot.com/activate?code=HA-XXXX-XXXX
   如果页面要求输入授权码，请输入: HA-XXXX-XXXX
   ```

5. 打开授权链接，登录 elianiot，点击确认授权。
6. Agent 自动取得长期身份并上线。

Add-on 会自动使用 Supervisor 地址和令牌，不需要填写 HA 地址、HA Token 或云端地址。

### Home Assistant Container / Docker

Home Assistant Container 没有 Add-on Store，需要单独运行 Agent。先创建 HA 长期访问令牌，然后执行：

```bash
docker run -d \
  --name elian-ha-agent \
  --restart unless-stopped \
  --network host \
  -e ELIAN_CLOUD_URL=wss://elianiot.com/api/v1/agent/connect \
  -e ELIAN_HA_URL=http://<HA地址>:8123 \
  -e ELIAN_HA_TOKEN=<HA长期访问令牌> \
  -e ELIAN_STATE_DIR=/data \
  -v elian-agent-data:/data \
  ghcr.io/fjiohj/elian-ha-agent:latest
```

请把 `<HA地址>` 和 `<HA长期访问令牌>` 替换为实际值。容器启动后日志会显示授权链接和短码，打开链接登录平台确认即可。

### 本地运行

```bash
cp agent.example.yaml agent.yaml
# 填写 ha_token；pair_code 留空即可走零配置授权
go run ./cmd/elian-agent --config agent.yaml
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `ELIAN_CLOUD_URL` | Agent Hub WebSocket 地址 |
| `ELIAN_PAIR_CODE` | 可选：兼容旧版一次性绑定码 |
| `ELIAN_AGENT_ID` | 配对后 Agent ID |
| `ELIAN_AGENT_SECRET` | 配对后长期密钥 |
| `ELIAN_HA_URL` | Home Assistant 地址 |
| `ELIAN_HA_TOKEN` | HA Long-Lived Access Token |
| `ELIAN_STATE_DIR` | Agent 状态目录 |

授权成功后，`agent_id` 和 `agent_secret` 保存到：

```text
state_dir/agent-state.yaml
```

如需重新绑定到另一个 elianiot 账号，请先停止 Agent 并删除该状态文件；再次启动后会显示新的设备授权链接。状态文件包含长期身份，请勿上传或分享。
