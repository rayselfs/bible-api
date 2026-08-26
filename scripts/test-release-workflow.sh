#!/usr/bin/env bash
set -euo pipefail

ci=.github/workflows/ci.yml
release=.github/workflows/release.yml
dockerfile=Dockerfile

test ! -e .azure-devops/azure-pipelines.yml
test -f "$ci"
test -f "$release"
test -f "$dockerfile"

grep -q 'pull_request:' "$ci"
grep -q 'workflow_dispatch:' "$ci"
grep -q 'contents: read' "$ci"
grep -q 'cancel-in-progress: true' "$ci"
grep -q 'actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1' "$ci"
grep -q 'actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e' "$ci"
grep -q 'actions/setup-node@395ad3262231945c25e8478fd5baf05154b1d79f' "$ci"
grep -q 'node-version: 24.x' "$ci"
grep -q 'npx --yes @redocly/cli@2.47.0 lint docs/openapi.yaml' "$ci"
grep -q 'go test -race ./... -count=1 -p=1' "$ci"
grep -q 'go vet ./...' "$ci"
grep -q './scripts/test-release-workflow.sh' "$ci"
grep -q 'docker build -t bible-api:verify .' "$ci"

grep -q '^  push:' "$release"
grep -q 'branches: \[main\]' "$release"
grep -q '      - \.dockerignore' "$release"
grep -q 'workflow_dispatch:' "$release"
grep -q 'deploy-bible-api-production' "$release"
grep -Fq "github.event_name == 'push' && 'deploy-bible-api-production' || inputs.confirmation" "$release"
grep -q 'environment: production' "$release"
grep -q 'id-token: write' "$release"
grep -q 'actions/setup-node@395ad3262231945c25e8478fd5baf05154b1d79f' "$release"
grep -q 'node-version: 24.x' "$release"
grep -q 'npx --yes @redocly/cli@2.47.0 lint docs/openapi.yaml' "$release"
grep -q 'go test -race ./... -count=1 -p=1' "$release"
grep -q 'go vet ./...' "$release"
grep -q './scripts/test-release-workflow.sh' "$release"
grep -q 'AZURE_CLIENT_ID: \${{ vars.AZURE_CLIENT_ID }}' "$release"
grep -q 'AZURE_TENANT_ID: \${{ vars.AZURE_TENANT_ID }}' "$release"
grep -q 'AZURE_SUBSCRIPTION_ID: \${{ vars.AZURE_SUBSCRIPTION_ID }}' "$release"
grep -q 'azure/login@532459ea530d8321f2fb9bb10d1e0bcf23869a43' "$release"
grep -q 'IMAGE_TAG=main-\${GITHUB_SHA::7}' "$release"
grep -q 'IMAGE_REF=\${ACR_LOGIN_SERVER}/\${IMAGE_REPOSITORY}@\${digest}' "$release"
grep -q 'PREVIOUS_READY_REVISION=' "$release"
grep -q 'PREVIOUS_IMAGE_REF=' "$release"
grep -q 'DAPR_APP_ID: bible-api' "$release"
grep -q 'properties.configuration.dapr.appId' "$release"
grep -q 'az containerapp update -g "\$RESOURCE_GROUP" -n "\$API_APP_NAME" --image "\$IMAGE_REF"' "$release"
test "$(grep -Fc 'az containerapp update' "$release")" -eq 1
grep -q 'az containerapp revision copy' "$release"
grep -q -- '--from-revision "\$PREVIOUS_READY_REVISION"' "$release"
grep -q -- '--image "\$PREVIOUS_IMAGE_REF"' "$release"
grep -q './scripts/smoke-release.sh' "$release"
grep -q '/v1.0/invoke/bible-api/method/health' scripts/smoke-release.sh

if grep -Eq 'az deployment|terraform|containerapp job|--set-env-vars|--ingress|client-secret|password:' "$release"; then
  echo 'Bible release must update only the existing Container App image with OIDC' >&2
  exit 1
fi

if grep -q 'docs/\*\*\|\.github/workflows/ci.yml' "$release"; then
  echo 'docs-only and CI-only changes must not trigger the production release' >&2
  exit 1
fi

dockerfile_invalid=0
if grep -q '^FROM --platform=' "$dockerfile"; then
  echo 'Dockerfile must not use a dynamic FROM platform expression' >&2
  dockerfile_invalid=1
fi
if grep -q 'github.com/swaggo/swag/cmd/swag@latest' "$dockerfile"; then
  echo 'Dockerfile must pin the Swag generator version' >&2
  dockerfile_invalid=1
fi
test "$dockerfile_invalid" -eq 0
test "$(grep -Ec '^FROM ' "$dockerfile")" -eq 2
grep -q '^FROM golang:1\.24\.6-alpine AS build$' "$dockerfile"
grep -q 'github.com/swaggo/swag/cmd/swag@v1\.16\.6' "$dockerfile"
grep -q 'swag init' "$dockerfile"
grep -q 'CGO_ENABLED=0' "$dockerfile"
grep -q 'GOOS=linux' "$dockerfile"
grep -q '^FROM gcr.io/distroless/static-debian12:nonroot$' "$dockerfile"
grep -q '^USER nonroot:nonroot$' "$dockerfile"
grep -Fq 'ENTRYPOINT ["/bible-api"]' "$dockerfile"

grep -Fq 'commit: ${{ steps.release_outputs.outputs.commit }}' "$release"
grep -Fq 'image: ${{ steps.release_outputs.outputs.image }}' "$release"
grep -q '^  publish_openapi:' "$release"
grep -q 'CONTAINER: api-docs-bible-api' "$release"
grep -Fq 'API_DOCS_AZURE_CLIENT_ID: ${{ vars.API_DOCS_AZURE_CLIENT_ID }}' "$release"
grep -Fq 'RELEASE_COMMIT: ${{ needs.deploy.outputs.commit }}' "$release"
grep -Fq 'RELEASE_IMAGE: ${{ needs.deploy.outputs.image }}' "$release"
grep -Fq 'inputs.fail_openapi_before_pointer && github.run_attempt == 1' "$release"

publish_job="$(sed -n '/^  publish_openapi:/,$p' "$release")"
grep -q 'needs: deploy' <<< "$publish_job"
grep -q 'environment: production' <<< "$publish_job"
grep -q 'id-token: write' <<< "$publish_job"
grep -q 'specs/${GITHUB_SHA}/openapi.yaml' <<< "$publish_job"
grep -q -- '--overwrite false' <<< "$publish_job"
grep -q -- '--name current.json' <<< "$publish_job"
grep -q -- '--overwrite true' <<< "$publish_job"

workflow_body="$(sed -n '/^          spec_blob="specs\//,$p' "$release" | sed 's/^          //')"
run_publication_case() {
  local pointer_json="$1"
  local candidate_run_id="$2"
  local expected="$3"
  local failure_injection="${4:-false}"
  local spec_fixture="${5:-missing}"
  local case_dir output status pointer_uploaded=false
  case_dir="$(mktemp -d)"
  mkdir -p "$case_dir/pointer"
  ln -s "$PWD/docs/openapi.yaml" "$case_dir/docs-openapi.yaml"
  if [[ "$pointer_json" != missing ]]; then
    printf '%s\n' "$pointer_json" > "$case_dir/pointer/current.json"
    cp "$case_dir/pointer/current.json" "$case_dir/expected-current.json"
  fi
  if [[ "$spec_fixture" != missing ]]; then
    mkdir -p "$case_dir/blobs/specs/0123456789abcdef0123456789abcdef01234567"
    if [[ "$spec_fixture" == identical ]]; then
      cp docs/openapi.yaml "$case_dir/blobs/specs/0123456789abcdef0123456789abcdef01234567/openapi.yaml"
    else
      printf 'different spec\n' > "$case_dir/blobs/specs/0123456789abcdef0123456789abcdef01234567/openapi.yaml"
    fi
  fi

  if output="$(POINTER_CASE_DIR="$case_dir" WORKFLOW_BODY="$workflow_body" GITHUB_RUN_ID="$candidate_run_id" GITHUB_SHA=0123456789abcdef0123456789abcdef01234567 GITHUB_REPOSITORY=rayselfs/bible-api RELEASE_COMMIT=0123456789abcdef0123456789abcdef01234567 RELEASE_IMAGE=alive.azurecr.io/alive/bible-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef FAIL_OPENAPI_BEFORE_POINTER="$failure_injection" bash -e -c '
    az() {
      command="$1 $2 $3"; name=""; file=""; overwrite=""
      while [ "$#" -gt 0 ]; do
        case "$1" in
          --name) name="$2"; shift 2 ;;
          --file) file="$2"; shift 2 ;;
          --overwrite) overwrite="$2"; shift 2 ;;
          *) shift ;;
        esac
      done
      blob="$POINTER_CASE_DIR/pointer/current.json"
      [ "$name" = current.json ] || blob="$POINTER_CASE_DIR/blobs/$name"
      case "$command" in
        "storage blob exists") if [ -f "$blob" ]; then printf true; else printf false; fi ;;
        "storage blob download") cp "$blob" "$file" ;;
        "storage blob upload")
          if [ -e "$blob" ] && [ "$overwrite" = false ]; then return 1; fi
          mkdir -p "$(dirname "$blob")"; cp "$file" "$blob"
          if [ "$name" = current.json ]; then printf "%s\n" pointer >> "$POINTER_CASE_DIR/uploads"; else printf "%s\n" spec >> "$POINTER_CASE_DIR/uploads"; fi
          ;;
      esac
    }
    cd "$POINTER_CASE_DIR"; mkdir -p docs; ln -s ../docs-openapi.yaml docs/openapi.yaml
    eval "$WORKFLOW_BODY"
  ' 2>&1)"; then status=0; else status=$?; fi
  [[ ! -e "$case_dir/uploads" ]] || ! grep -Fxq pointer "$case_dir/uploads" || pointer_uploaded=true

  case "$expected" in
    upload) [[ $status -eq 0 && $pointer_uploaded == true ]] ;;
    noop) [[ $status -eq 0 && $pointer_uploaded == false ]] && cmp "$case_dir/expected-current.json" "$case_dir/pointer/current.json" ;;
    invalid-pointer) [[ $status -ne 0 && $pointer_uploaded == false ]] && cmp "$case_dir/expected-current.json" "$case_dir/pointer/current.json" && grep -Fq 'Invalid existing API docs pointer' <<< "$output" ;;
    invalid-candidate) [[ $status -ne 0 && $pointer_uploaded == false ]] && grep -Fq 'Invalid GITHUB_RUN_ID' <<< "$output" ;;
    pre-pointer-failure) [[ $status -ne 0 && $pointer_uploaded == false ]] && grep -Fq 'Requested failure before API docs pointer upload' <<< "$output" ;;
    spec-idempotent) [[ $status -eq 0 && $pointer_uploaded == true ]] && { [[ ! -e "$case_dir/uploads" ]] || ! grep -Fxq spec "$case_dir/uploads"; } ;;
    spec-mismatch) [[ $status -ne 0 && $pointer_uploaded == false ]] && grep -Fq 'Existing OpenAPI spec hash does not match' <<< "$output" ;;
  esac
  rm -rf "$case_dir"
}

valid_pointer='{"releaseUrl":"https://github.com/rayselfs/bible-api/actions/runs/20"}'
run_publication_case missing 20 upload
run_publication_case missing 20 pre-pointer-failure true
run_publication_case "$valid_pointer" 21 spec-idempotent false identical
run_publication_case "$valid_pointer" 21 spec-mismatch false different
run_publication_case "$valid_pointer" 19 noop
run_publication_case "$valid_pointer" 20 noop
run_publication_case "$valid_pointer" 21 upload
run_publication_case '{' 22 invalid-pointer
run_publication_case '{"releaseUrl":"https://github.com/rayselfs/bible-api/actions/runs/99999999999999999999"}' 100000000000000000000 upload
run_publication_case missing 0 invalid-candidate
run_publication_case missing 01 invalid-candidate

smoke_line="$(grep -n 'name: Smoke Bible API release' "$release" | cut -d: -f1)"
outputs_line="$(grep -n 'id: release_outputs' "$release" | cut -d: -f1)"
rollback_line="$(grep -n 'name: Roll back failed runtime' "$release" | cut -d: -f1)"
publish_line="$(grep -n '^  publish_openapi:' "$release" | cut -d: -f1)"
[[ "$smoke_line" -lt "$outputs_line" && "$outputs_line" -lt "$rollback_line" && "$rollback_line" -lt "$publish_line" ]]
