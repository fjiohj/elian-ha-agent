#!/bin/sh
set -eu

OPTIONS=/data/options.json
CONFIG=/data/agent.yaml

# Add-on 模式下无需用户填写云端地址、HA 地址或 HA Token：
# - 云端地址固定为生产 Agent Hub；
# - HA 地址通过 Supervisor 内网 API；
# - HA Token 使用 Supervisor 自动注入的 SUPERVISOR_TOKEN。
CLOUD_URL="${ELIAN_CLOUD_URL:-wss://elianiot.com/api/v1/agent/connect}"
PAIR_CODE=""
if [ -f "$OPTIONS" ]; then
  PAIR_CODE=$(jq -r '.pair_code // ""' "$OPTIONS")
fi
HA_URL="${ELIAN_HA_URL:-http://supervisor/core}"
HA_TOKEN="${SUPERVISOR_TOKEN:-}"
HEARTBEAT=30

if [ -z "$HA_TOKEN" ]; then
  echo "ERROR: 未获取到 SUPERVISOR_TOKEN，请确认这是 Home Assistant Add-on 运行环境" >&2
  exit 1
fi

jq -n \
  --arg cloud_url "$CLOUD_URL" \
  --arg pair_code "$PAIR_CODE" \
  --arg ha_url "$HA_URL" \
  --arg ha_token "$HA_TOKEN" \
  --argjson heartbeat_seconds "$HEARTBEAT" \
  '{
    cloud_url: $cloud_url,
    pair_code: $pair_code,
    ha_url: $ha_url,
    ha_token: $ha_token,
    state_dir: "/data",
    heartbeat_seconds: $heartbeat_seconds,
    reconnect_min_seconds: 2,
    reconnect_max_seconds: 60
  }' > "$CONFIG"

chmod 600 "$CONFIG"
exec /usr/local/bin/elian-agent --config "$CONFIG"
