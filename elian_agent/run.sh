#!/bin/sh
set -eu

OPTIONS=/data/options.json
CONFIG=/data/agent.yaml

CLOUD_URL=$(jq -r '.cloud_url // "wss://elianiot.com/api/v1/agent/connect"' "$OPTIONS")
PAIR_CODE=$(jq -r '.pair_code // ""' "$OPTIONS")
HA_URL=$(jq -r '.ha_url // "http://supervisor/core"' "$OPTIONS")
HA_TOKEN=$(jq -r '.ha_token // ""' "$OPTIONS")
HEARTBEAT=$(jq -r '.heartbeat_seconds // 30' "$OPTIONS")

if [ -z "$HA_TOKEN" ] && [ -n "${SUPERVISOR_TOKEN:-}" ]; then
  HA_TOKEN="$SUPERVISOR_TOKEN"
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
