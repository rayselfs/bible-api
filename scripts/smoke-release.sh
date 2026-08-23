#!/usr/bin/env bash
set -euo pipefail

: "${RESOURCE_GROUP:?}"
: "${API_GATEWAY_APP_NAME:?}"

output="$(timeout 60s script -q -e -c \
  "az containerapp exec -g \"$RESOURCE_GROUP\" -n \"$API_GATEWAY_APP_NAME\" --command \"/usr/bin/wget -S -O - http://localhost:3500/v1.0/invoke/bible-api/method/health\"" \
  /dev/null 2>&1)"
output="${output//$'\r'/}"
printf '%s\n' "$output"
grep -Eq 'HTTP/1\.[01][[:space:]]+200' <<<"$output"
grep -Eq '"status"[[:space:]]*:[[:space:]]*"UP"' <<<"$output"
