#!/usr/bin/env bash
set -euo pipefail

[[ "$GITHUB_REF" == refs/heads/main ]] || exit 1
[[ "$RESOURCE_GROUP" == alive && "$API_APP_NAME" == bible-api && "$DAPR_APP_ID" == bible-api ]] || exit 1
case "$RELEASE_TARGET" in
  api)
    [[ "$RELEASE_CONFIRMATION" == deploy-bible-api-production ]] || exit 1
    client_id="${AZURE_CLIENT_ID:-}"
    ;;
  import)
    [[ "$GITHUB_EVENT_NAME" == workflow_dispatch ]] || exit 1
    [[ "$RELEASE_CONFIRMATION" == prepare-bible-import-image && "$IMPORT_JOB_NAME" == bible-import ]] || exit 1
    client_id="${BIBLE_IMPORT_AZURE_CLIENT_ID:-}"
    # Missing import credentials must never fall back to the production API identity.
    [[ -n "$client_id" && "$client_id" != "${AZURE_CLIENT_ID:-}" ]] || exit 1
    ;;
  *) echo 'Unknown release target' >&2; exit 1 ;;
esac
for name in client_id AZURE_TENANT_ID AZURE_SUBSCRIPTION_ID; do
  test -n "${!name:-}" || { echo "Missing release configuration: $name" >&2; exit 1; }
done
case "${1:-}" in
  validate) printf 'client_id=%s\n' "$client_id" ;;
  prepare-import)
    [[ "$RELEASE_TARGET" == import ]] || exit 1
    [[ "$IMAGE_REF" =~ ^alive\.azurecr\.io/alive/bible-api@sha256:[a-f0-9]{64}$ ]] || exit 1
    az containerapp job update -g "$RESOURCE_GROUP" -n "$IMPORT_JOB_NAME" --image "$IMAGE_REF" --only-show-errors --output none
    ;;
  *) exit 1 ;;
esac
