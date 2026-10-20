#!/usr/bin/env bash
set -euo pipefail
export GIN_MODE=test
cd "$(dirname "$0")/.."
# Never accept production credentials. The PG suite separately requires a loopback DSN.

work="$(mktemp -d)"
fixture="bible-readiness-$$-${RANDOM}"
network="${fixture}-net"
dapr="${fixture}-dapr"
cleanup() {
  result=$?
  if [[ "$result" -ne 0 ]]; then
    echo "Isolation fixture failed; test-owned container logs follow" >&2
    docker inspect --format '{{json .State}} {{json .NetworkSettings.Ports}}' "$fixture" >&2 2>/dev/null || true
    docker logs "$fixture" >&2 || true
    docker logs "$dapr" >&2 || true
  fi
  if [[ -n "${monitor:-}" ]]; then touch "$work/monitor-stop"; wait "$monitor" 2>/dev/null || true; fi
  docker rm -f "$dapr" "$fixture" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
GOMAXPROCS=2 go test -c -p=1 -o "$work/server.test" ./internal/server
GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -p=1 -o "$work/port-fixture" ./scripts/readiness-fixture
docker network create "$network" >/dev/null
docker run -d --name "$fixture" --platform linux/amd64 --network "$network" \
  --cpus 0.5 --memory 128m -p 127.0.0.1::3500 -p 127.0.0.1::8081 \
  -v "$work/port-fixture:/fixture:ro" --entrypoint /fixture golang:1.24.6-alpine >/dev/null
port="$(docker port "$fixture" 3500/tcp | sed 's/.*://')"
control="$(docker port "$fixture" 8081/tcp | sed 's/.*://')"
for attempt in {1..30}; do
  if curl -s -o /dev/null --max-time 1 "http://127.0.0.1:$control/release"; then break; fi
  sleep 1
done
BIBLE_TEST_DAPR_HTTP_PORT="$port" BIBLE_DAPR_CONFLICT_MONITOR="$work/monitor-stop" \
  "$work/server.test" -test.run '^TestReadinessConflictNeverReady$' -test.count=1 >"$work/monitor.log" 2>&1 &
monitor=$!
for attempt in {1..30}; do
  [[ -f "$work/monitor-stop.started" ]] && break
  sleep 1
done
[[ -f "$work/monitor-stop.started" ]]
docker run -d --name "$dapr" --platform linux/amd64 --network "container:$fixture" \
  --cpus 0.5 --memory 256m --entrypoint /daprd \
  mcr.microsoft.com/dapr-capps/daprd@sha256:e99b96b58ef5461561111daaba7770df1ddf723dbbeaa3bc308546c808799cbe \
  --app-id bible-readiness-fixture --app-port 0 --dapr-http-port 3500 --dapr-grpc-port 50001 \
  --dapr-internal-grpc-port 50002 --enable-metrics=false >/dev/null
for attempt in {1..30}; do
  [[ "$(docker inspect -f '{{.State.Running}}' "$dapr")" == false ]] && break
  sleep 1
done
[[ "$(docker inspect -f '{{.State.Running}}' "$dapr")" == false ]]
docker logs "$dapr" > "$work/conflict.log" 2>&1
grep -Eq '(50002.*address already in use|address already in use.*50002)' "$work/conflict.log"
touch "$work/monitor-stop"
if ! wait "$monitor"; then cat "$work/monitor.log" >&2; exit 1; fi
BIBLE_TEST_DAPR_HTTP_PORT="$port" BIBLE_TEST_DAPR_EXPECTED_STATUS=503 \
  "$work/server.test" -test.run '^TestReadinessRealDapr$' -test.count=1
curl -fsS --max-time 2 -X POST "http://127.0.0.1:$control/release" >/dev/null
docker start "$dapr" >/dev/null
healthy() {
  for attempt in {1..60}; do
    [[ "$(curl -s -o /dev/null --max-time 1 -w '%{http_code}' "http://127.0.0.1:$port/v1.0/healthz/outbound")" == 204 ]] && return 0
    sleep 1
  done
  echo 'Isolated Dapr did not become healthy' >&2
  return 1
}
healthy
BIBLE_TEST_DAPR_HTTP_PORT="$port" BIBLE_TEST_DAPR_EXPECTED_STATUS=200 \
  "$work/server.test" -test.run '^TestReadinessRealDapr$' -test.count=1
docker stop -t 5 "$dapr" >/dev/null
BIBLE_TEST_DAPR_HTTP_PORT="$port" BIBLE_TEST_DAPR_EXPECTED_STATUS=503 \
  "$work/server.test" -test.run '^TestReadinessRealDapr$' -test.count=1
docker start "$dapr" >/dev/null
healthy
BIBLE_TEST_DAPR_HTTP_PORT="$port" BIBLE_TEST_DAPR_EXPECTED_STATUS=200 \
  "$work/server.test" -test.run '^TestReadinessRealDapr$' -test.count=1
echo 'Isolated port conflict, sidecar loss and recovery passed; ACA handoff remains a separate live gate.'
