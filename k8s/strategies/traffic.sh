#!/usr/bin/env bash
set -euo pipefail

count=${1:-0}
if (( $# > 1 )) || [[ $count != --forward-only && ! $count =~ ^(0|[1-9][0-9]*)$ ]]; then
  echo "Usage: $0 [--forward-only | REQUEST_COUNT]" >&2
  exit 2
fi

port=${SHORTY_PORT:-18080}
profile=${SHORTY_PROFILE:-minikube}
eg=(kubectl --context "$profile" -n envoy-gateway-system)
k=(kubectl --context "$profile" -n shorty-rollouts)
url="http://shorty-rollouts.localhost:$port"
source "$(dirname "$0")/probes.sh"

service=$("${eg[@]}" get svc \
  -l 'gateway.envoyproxy.io/owning-gateway-namespace=shorty-rollouts,gateway.envoyproxy.io/owning-gateway-name=shorty-rollouts' \
  -o json | jq -r '[.items[] | select(any(.spec.ports[]?; .port == 80))][0].metadata.name // empty')
if [[ -z $service ]]; then
  echo "Envoy service for Gateway shorty-rollouts was not found. Run setup.sh first." >&2
  exit 1
fi

forward=("${eg[@]}" port-forward "svc/$service" "$port:80")
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

active_mode() {
  "${k[@]}" --request-timeout=2s get httproute shorty-api -o json | jq -r '
    if any(.spec.rules[]?.filters[]?; .type == "RequestMirror") then "shadow"
    elif any(.spec.rules[]?.matches[]?.headers[]?;
      (.name | ascii_downcase) == "x-user-group" and .value == "beta") then "ab"
    else "default"
    end'
}

print_response() {
  local label=$1 line
  shift
  if line=$(response "$@"); then
    printf '%s | %s%s\n' "$(date +%T)" "$label" "$line"
  else
    printf '%s | %srequest failed\n' "$(date +%T)" "$label"
    failures=$((failures + 1))
  fi
}

echo "GET $url/api/version (Ctrl+C to stop)"
failures=0
mode=default
next_mode_check=0
for ((i=0; count == 0 || i<count; i++)); do
  if ! kill -0 "$pid" 2>/dev/null; then
    cat "$log" >&2
    exit 1
  fi
  if (( count == 0 && SECONDS >= next_mode_check )); then
    next_mode_check=$((SECONDS + 2))
    if observed_mode=$(active_mode) && [[ $mode != "$observed_mode" ]]; then
      mode=$observed_mode
      if [[ $mode == shadow ]]; then
        printf '%s | Shadow: checking the mirror with X-Demo-ID\n' "$(date +%T)"
        shadow_proof 5 || failures=$((failures + 1))
        # Leave the correlated pod logs visible before resuming the stream.
        sleep 6
      fi
    fi
  fi
  case "$mode" in
    ab)
      print_response 'A/B x-user-group omitted | '
      print_response 'A/B x-user-group: beta | ' -H 'x-user-group: beta'
      ;;
    shadow) print_response 'Shadow client | ' ;;
    *) print_response '' ;;
  esac
  sleep "${RATE:-0.5}"
done
(( failures == 0 ))
