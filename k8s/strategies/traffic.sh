#!/usr/bin/env bash
set -euo pipefail

count=${1:-0}
if (( $# > 1 )) || [[ $count != --forward-only && ! $count =~ ^(0|[1-9][0-9]*)$ ]]; then
  echo "Usage: $0 [--forward-only | REQUEST_COUNT]" >&2
  exit 2
fi

port=${SHORTY_PORT:-8080}
k=(kubectl --context "${SHORTY_PROFILE:-shorty-rollouts}" -n envoy-gateway-system)
service=$("${k[@]}" get svc \
  -l 'gateway.envoyproxy.io/owning-gateway-namespace=shorty,gateway.envoyproxy.io/owning-gateway-name=shorty' \
  -o json | jq -r '[.items[] | select(any(.spec.ports[]?; .port == 80))][0].metadata.name // empty')
if [[ -z $service ]]; then
  echo "Envoy service for Gateway shorty was not found. Run setup.sh first." >&2
  exit 1
fi

forward=("${k[@]}" port-forward "svc/$service" "$port:80")
if [[ $count == --forward-only ]]; then
  exec "${forward[@]}"
fi

log=$(mktemp)
"${forward[@]}" >"$log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -f "$log"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

for ((attempt=0; attempt<30; attempt++)); do
  if ! kill -0 "$pid" 2>/dev/null || grep -q '^Forwarding from' "$log"; then break; fi
  sleep 0.5
done
cat "$log"
if ! kill -0 "$pid" 2>/dev/null || ! grep -q '^Forwarding from' "$log"; then
  echo "Envoy port-forward did not start." >&2
  exit 1
fi

url="http://shorty.localhost:$port/api/version"
echo "GET $url (Ctrl+C to stop)"
failures=0
for ((i=0; count == 0 || i<count; i++)); do
  if ! kill -0 "$pid" 2>/dev/null; then
    cat "$log" >&2
    exit 1
  fi
  if response=$(curl --noproxy '*' --resolve "shorty.localhost:$port:127.0.0.1" \
    --max-time 5 -fsS "$url" | jq -er 'select(.version and .pod) | "\(.version)  \(.pod)"'); then
    printf '%s | %s\n' "$(date +%T)" "$response"
  else
    printf '%s | request failed\n' "$(date +%T)"
    failures=$((failures + 1))
  fi
  sleep "${RATE:-0.5}"
done
(( failures == 0 ))
