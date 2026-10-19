#!/usr/bin/env bash
set -euo pipefail
: "${RESOURCE_GROUP:?}"
: "${API_GATEWAY_APP_NAME:?}"
exec python3 "$(dirname "$0")/smoke-release.py"
