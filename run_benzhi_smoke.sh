#!/usr/bin/env bash
# Deterministic smoke test: builds the server, starts it against a temporary
# SQLite database, drives the real HTTP API (configuration freeze, trial
# creation, startup, sampling), and verifies responses — all without external
# network access. Every curl response is captured into a variable before being
# asserted (never piped into grep), and every process and temporary file is
# cleaned up on exit.
set -euo pipefail

WORKDIR="$(mktemp -d)"
DB="$WORKDIR/smoke.db"
BIN="$WORKDIR/server"
PORT="${SMOKE_PORT:-18080}"
BASE="http://127.0.0.1:$PORT"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

extract_field() {
  # $1 = json text, $2 = field name; returns the first string value.
  printf '%s' "$1" | grep -o "\"$2\":\"[^\"]*\"" | head -1 | cut -d'"' -f4
}

CONFIG='{"chambers":[{"id":"c-main","name":"主承压舱段","volume_ul":1000},{"id":"c-end","name":"端盖舱段","volume_ul":500}],"ports":[{"id":"p-inlet","chamber":"c-main","kind":"pressure_inlet"},{"id":"p-sensor","chamber":"c-main","kind":"pressure_sensor","channel":"ch-1"},{"id":"p-temp","chamber":"c-end","kind":"temperature_sensor","channel":"ch-2"}],"pipes":[{"id":"pipe-1","from":"p-sensor","to":"p-temp"}],"seal_boundaries":[{"id":"s-1","chamber":"c-main","checks":["外观检查","密封复查"]}],"steps":[{"index":1,"target_pa":5000000,"ramp_up_pa_per_s":100000,"ramp_down_pa_per_s":100000,"hold_ms":600000,"leak_limit_ul_per_s":10,"max_drop_pa":50000},{"index":2,"target_pa":10000000,"ramp_up_pa_per_s":100000,"ramp_down_pa_per_s":100000,"hold_ms":600000,"leak_limit_ul_per_s":10,"max_drop_pa":50000}],"calibrations":[{"channel":"ch-1","serial":"SN-P","expires_at_ms":2000000000000,"summary":"压力"},{"channel":"ch-2","serial":"SN-T","expires_at_ms":2000000000000,"summary":"温度"}],"compensation":{"ref_temp_mc":20000,"temp_coeff_ppm":10}}'

echo "building server ..."
go build -o "$BIN" ./cmd/server

echo "starting server on $PORT ..."
"$BIN" -addr "127.0.0.1:$PORT" -db "$DB" -frontend web >"$WORKDIR/server.log" 2>&1 &
SERVER_PID=$!

# Wait for the health endpoint to come up.
ready=0
for _ in $(seq 1 100); do
  if HEALTH="$(curl -sf "$BASE/health" 2>/dev/null)"; then
    ready=1
    break
  fi
  sleep 0.05
done
if [[ "$ready" != "1" ]]; then
  echo "server did not become healthy" >&2
  cat "$WORKDIR/server.log" >&2
  exit 1
fi

HEALTH="$(curl -s "$BASE/health")"
[[ "$HEALTH" == *'"ok"'* ]] || { echo "health check failed: $HEALTH" >&2; exit 1; }
echo "health ok"

# Freeze configuration.
FREEZE="$(curl -s -X POST -H 'Content-Type: application/json' -d "$CONFIG" "$BASE/api/v1/configurations/freeze")"
DIGEST="$(extract_field "$FREEZE" digest)"
[[ -n "$DIGEST" ]] || { echo "freeze failed: $FREEZE" >&2; exit 1; }
echo "configuration frozen: $DIGEST"

# Create a trial.
TRIAL="$(curl -s -X POST -H 'Content-Type: application/json' -d "{\"config_digest\":\"$DIGEST\"}" "$BASE/api/v1/trials")"
TRIAL_ID="$(extract_field "$TRIAL" id)"
[[ -n "$TRIAL_ID" ]] || { echo "create trial failed: $TRIAL" >&2; exit 1; }
echo "trial created: $TRIAL_ID"

# Atomic startup: bind components and lease resources.
STARTUP_BODY='{"bindings":[{"serial":"SN-P","type":"pressure_sensor","position":"p-sensor"}],"leases":[{"resource_id":"chamber-1"},{"resource_id":"pump-1"},{"resource_id":"collector-1"},{"resource_id":"valve-1"}]}'
STARTUP="$(curl -s -X POST -H 'Content-Type: application/json' -d "$STARTUP_BODY" "$BASE/api/v1/trials/$TRIAL_ID/startup")"
[[ "$STARTUP" == *'"ok":true'* ]] || { echo "startup failed: $STARTUP" >&2; exit 1; }
echo "startup ok"

# Submit a sample.
SAMPLE="$(curl -s -X POST -H 'Content-Type: application/json' -d '{"logical_ms":2000,"pressure_pa":5000000,"temp_mc":20000}' "$BASE/api/v1/trials/$TRIAL_ID/samples")"
[[ "$SAMPLE" == *'"ok":true'* ]] || { echo "sample failed: $SAMPLE" >&2; exit 1; }
echo "sample ok"

# Read the evidence chain.
EVIDENCE="$(curl -s "$BASE/api/v1/trials/$TRIAL_ID/evidence")"
[[ "$EVIDENCE" == *'"samples"'* ]] || { echo "evidence failed: $EVIDENCE" >&2; exit 1; }
echo "evidence ok"

# Frontend entry is served by the Go process.
PAGE="$(curl -s "$BASE/")"
[[ "$PAGE" == *'压力舱'* ]] || { echo "frontend not served" >&2; exit 1; }
echo "frontend ok"

echo "smoke test passed"
