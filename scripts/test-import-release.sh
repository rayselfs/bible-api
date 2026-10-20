#!/usr/bin/env bash
set -euo pipefail
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
mkdir "$fixture/bin"
cat > "$fixture/bin/az" <<'AZ'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$AZ_LOG"
[[ "$*" == "containerapp job update -g alive -n bible-import --image alive.azurecr.io/alive/bible-api@sha256:"*" --only-show-errors --output none" ]] || exit 91
AZ
chmod +x "$fixture/bin/az"
export PATH="$fixture/bin:$PATH" AZ_LOG="$fixture/az.log"
export GITHUB_EVENT_NAME=workflow_dispatch
export GITHUB_REF=refs/heads/main RELEASE_TARGET=import RELEASE_CONFIRMATION=prepare-bible-import-image
export RESOURCE_GROUP=alive API_APP_NAME=bible-api DAPR_APP_ID=bible-api IMPORT_JOB_NAME=bible-import
export AZURE_CLIENT_ID=production-id BIBLE_IMPORT_AZURE_CLIENT_ID=import-id AZURE_TENANT_ID=tenant AZURE_SUBSCRIPTION_ID=subscription
export IMAGE_REF=alive.azurecr.io/alive/bible-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
reject() {
  rm -f "$AZ_LOG"
  if env "$@" ./scripts/release-target.sh prepare-import > "$fixture/output" 2>&1; then echo "unsafe import preparation accepted: $*" >&2; exit 1; fi
  test ! -s "$AZ_LOG"
}
reject GITHUB_REF=refs/heads/feature
reject GITHUB_EVENT_NAME=push
reject API_APP_NAME=other-api
reject DAPR_APP_ID=other-dapr
reject RELEASE_TARGET=unknown
reject RELEASE_TARGET=api
reject RELEASE_CONFIRMATION=deploy-bible-api-production
reject IMPORT_JOB_NAME=other-job
reject RESOURCE_GROUP=other-group
reject BIBLE_IMPORT_AZURE_CLIENT_ID=
reject BIBLE_IMPORT_AZURE_CLIENT_ID=production-id
reject AZURE_TENANT_ID=
reject IMAGE_REF=alive.azurecr.io/alive/bible-api:main
reject IMAGE_REF=other.azurecr.io/alive/bible-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
rm -f "$AZ_LOG"
./scripts/release-target.sh validate > "$fixture/output"
grep -Fxq 'client_id=import-id' "$fixture/output"
test ! -s "$AZ_LOG"
AZURE_CLIENT_ID= ./scripts/release-target.sh validate > "$fixture/output"
grep -Fxq 'client_id=import-id' "$fixture/output"
test ! -s "$AZ_LOG"
./scripts/release-target.sh prepare-import
test "$(wc -l < "$AZ_LOG" | tr -d ' ')" -eq 1
grep -Fxq "containerapp job update -g alive -n bible-import --image $IMAGE_REF --only-show-errors --output none" "$AZ_LOG"
rm -f "$AZ_LOG"
RELEASE_TARGET=api RELEASE_CONFIRMATION=deploy-bible-api-production BIBLE_IMPORT_AZURE_CLIENT_ID= ./scripts/release-target.sh validate > "$fixture/output"
grep -Fxq 'client_id=production-id' "$fixture/output"
test ! -s "$AZ_LOG"
GITHUB_EVENT_NAME=push RELEASE_TARGET=api RELEASE_CONFIRMATION=deploy-bible-api-production ./scripts/release-target.sh validate > "$fixture/output"
grep -Fxq 'client_id=production-id' "$fixture/output"
test ! -s "$AZ_LOG"
echo 'Import release target fixtures passed' 
