#!/bin/sh
set -eu

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
