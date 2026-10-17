# elian HA Agent

将家庭中的 Home Assistant 安全接入 elianiot 平台。

Agent 由家庭侧主动连接云端，不需要公网 IP、端口映射、FRP、Tailscale 或 Cloudflare Tunnel。

## 适用环境

- Home Assistant OS
- Home Assistant Supervised

Home Assistant Container 没有 Add-on Store，请改用项目 README 中的 Docker 方式。

## 安装步骤

1. 在 Home Assistant 中进入「设置 → 加载项 → 加载项商店」。
2. 点击右上角三个点 →「仓库」，添加：

   ```text
   https://github.com/fjiohj/elian-ha-agent
   ```

3. 安装 **elian HA Agent**。
4. 启动 Add-on。
5. 打开 Add-on「日志」，会显示授权链接和短码，例如：

   ```text
   请在浏览器打开: https://elianiot.com/activate?code=HA-XXXX-XXXX
   如果页面要求输入授权码，请输入: HA-XXXX-XXXX
   ```

6. 打开链接，登录 elianiot，点击确认授权。
7. Agent 会自动取得身份并上线，不需要填写任何配置。

## 判断是否成功

Add-on 日志中出现以下内容表示授权成功：

```text
Agent 设备授权成功，agent_id=...
Agent 认证成功，agent_id=...
```

同时 elianiot 的「平台接入」页面会显示 Agent 在线。

## 关于设备授权

- 授权链接和短码 10 分钟内有效；
- 每个短码只能使用一次；
- 授权成功后长期身份保存在 `/data/agent-state.yaml`；
- 以后重启 Add-on 无需再次授权；
- 如需绑定到另一个平台账号，请停止 Add-on、删除 `/data/agent-state.yaml`，再次启动后会生成新的授权链接。
