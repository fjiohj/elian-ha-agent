# Agent 架构

## 连接模型

```text
Home Assistant
      ↕ REST / WebSocket
elian HA Agent
      ↕ 主动建立 WSS
elianiot Agent Hub
      ↕
elianiot 业务平台
```

用户网络不需要开放入站端口。Agent 只需要能够访问：

```text
wss://elianiot.com:443/api/v1/agent/connect
```

## 云端下发请求

```json
{
  "type": "request",
  "id": "req-001",
  "method": "GET",
  "path": "/api/states",
  "body": null
}
```

Agent 仅允许代理 HA `/api/` 路径，拒绝绝对 URL、路径穿越和非 HA API 请求。

## Agent 返回响应

```json
{
  "type": "response",
  "id": "req-001",
  "status": 200,
  "data": []
}
```

## 注册表只读 RPC

设备同步需要合并同一物理设备的多个实体。云端通过独立消息请求 HA 注册表：

```json
{
  "type": "ws_command",
  "id": "req-002",
  "command": "config/device_registry/list"
}
```

Agent 只允许以下三条只读命令，不接受任意 HA WebSocket 命令或参数：

```text
config/entity_registry/list
config/device_registry/list
config/area_registry/list
```

```json
{
  "type": "ws_result",
  "id": "req-002",
  "data": []
}
```

## 状态推送

Agent 登录 HA WebSocket 后订阅 `state_changed`：

```json
{
  "type": "event",
  "event": "state_changed",
  "time": 1760000000,
  "data": {
    "entity_id": "binary_sensor.door",
    "new_state": {
      "state": "on"
    }
  }
}
```

HA 原始事件信封为 `event.event_type` / `event.data`。Agent 只把 `state_changed` 的 data 转发给云端，云端再按 Agent 所属平台账号精确匹配设备。

## Agent 身份

首次启动默认使用 OAuth Device Authorization 风格的零配置授权：

```text
Agent 请求 device_code
  → 云端返回 user_code + 授权链接
  → 用户登录 elianiot 确认
  → Agent 轮询换取长期身份
```

Agent 不保存 user_code，云端数据库只保存 device_code/user_code 的 SHA-256 哈希。授权成功后云端返回：

```json
{
  "agent_id": 123,
  "agent_secret": "long-lived-secret"
}
```

Agent 将身份保存到 `state_dir/agent-state.yaml`，文件权限为 `0600`。

兼容模式仍支持一次性 `AGENT-XXXX-XXXX` 绑定码，适合自动化部署或旧版本迁移。

## 可靠性

- Agent 每 5~60 秒发送应用层心跳（默认 30 秒）；
- 云端 90 秒未收到任何消息会关闭半开连接并标记离线；
- Agent 使用带随机抖动的指数退避重连；
- 单 Agent 重连时，新会话会替换旧会话，旧会话不会把新会话误标离线。
