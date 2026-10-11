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

## Agent 身份

首次配对使用一次性绑定码。配对成功后云端返回：

```json
{
  "agent_id": 123,
  "agent_secret": "long-lived-secret"
}
```

Agent 将身份保存到 `state_dir/agent-state.yaml`，文件权限为 `0600`。
