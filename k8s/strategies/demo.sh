#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  baseline|rolling|blue-green|canary|ab|shadow) ;;
  *) echo "Usage: $0 {baseline|rolling|blue-green|canary|ab|shadow}" >&2; exit 2 ;;
esac

routes_dir="$(cd "$(dirname "$0")" && pwd)/routes"
k=(kubectl --context "${SHORTY_PROFILE:-shorty-rollouts}" -n shorty)
port=${SHORTY_PORT:-8080}
url="http://shorty.localhost:$port"
http() {
  curl --noproxy '*' --resolve "shorty.localhost:$port:127.0.0.1" --max-time 5 -fsS "$@"
}

fail() { echo "FAIL: $*" >&2; exit 1; }
step() { printf '\n[%s] %s\n' "$(date +%T)" "$*"; }
response() {
  http "$@" "$url/api/version" | jq -er 'select(.version == "v1" or .version == "v2") | "\(.version)  \(.pod)"'
}

route() {
  local name=$1 attempt
  step "HTTPRoute shorty-api ← routes/$name.yaml"
  "${k[@]}" patch httproute shorty-api --type=merge --patch-file "$routes_dir/$name.yaml"
  for ((attempt=0; attempt<60; attempt++)); do
    if "${k[@]}" get httproute shorty-api -o json | jq -e '
      .metadata.generation as $generation |
      any(.status.parents[]? | select(.parentRef.name == "shorty");
        ([.conditions[] | select(.observedGeneration == $generation and .status == "True") | .type]
          | index("Accepted") != null and index("ResolvedRefs") != null))' >/dev/null; then
      "${k[@]}" get httproute shorty-api -o json | jq -c '.spec.rules[]'
      return
    fi
    sleep 1
  done
  "${k[@]}" describe httproute shorty-api >&2
  fail "route was not accepted"
}

wait_version() {
  local expected=$1 line attempt
  shift
  for ((attempt=0; attempt<30; attempt++)); do
    if line=$(response "$@") && [[ ${line%% *} == "$expected" ]]; then
      printf '%s\n' "$line"
      return
    fi
    sleep 1
  done
  fail "expected $expected from Envoy Gateway"
}

sample() {
  local count=$1 lower=$2 upper=$3 line request v1=0 v2=0
  for ((request=0; request<count; request++)); do
    line=$(response) || return 1
    case "${line%% *}" in
      v1) v1=$((v1 + 1)) ;;
      v2) v2=$((v2 + 1)) ;;
    esac
  done
  printf '%s requests: v1=%s, v2=%s\n' "$count" "$v1" "$v2"
  (( v2 >= lower && v2 <= upper ))
}

reset_blue() {
  route blue
  "${k[@]}" set image deployment/api api=shorty-api:v1
  "${k[@]}" rollout status deployment/api --timeout=120s
  wait_version v1
}

case "$1" in
  baseline)
    step "Gateway, two ready replicas per release and shared PostgreSQL"
    reset_blue
    "${k[@]}" get gateway,httproute,deploy,pods
    "${k[@]}" get deployment/api -o json | jq '
      .spec.template.spec.containers[0] | {readinessProbe, livenessProbe}'
    http "$url/healthz" | jq -e 'select(.status == "ok")'
    http -o /dev/null "$url/"
    http -o /dev/null "$url/assets/app.js"
    # Collect multiple requests so both v1 pods are visible.
    replies=$(for ((i=0; i<12; i++)); do response || exit 1; done)
    counts=$(printf '%s\n' "$replies" | sort | uniq -c)
    printf '%s\n' "$counts"
    [[ $(wc -l <<<"$counts") -ge 2 ]] || fail "expected at least two serving v1 replicas"
    ;;
  rolling)
    reset_blue
    step "RollingUpdate: maxSurge=1, maxUnavailable=0; Service api stays selected"
    "${k[@]}" get deployment/api -o json | jq '.spec.strategy'
    "${k[@]}" set image deployment/api api=shorty-api:v2
    seen_v1=0
    seen_v2=0
    for ((i=0; i<30; i++)); do
      line=$(response)
      printf '%s  %s\n' "$(date +%T)" "$line"
      case "${line%% *}" in
        v1) seen_v1=1 ;;
        v2) seen_v2=1 ;;
      esac
      if (( i % 5 == 0 )); then
        "${k[@]}" get rs -l app.kubernetes.io/name=api -o wide
      fi
      sleep 1
    done
    "${k[@]}" rollout status deployment/api --timeout=120s
    (( seen_v1 && seen_v2 )) || fail "expected both versions during the rollout"
    sample 12 12 12 || fail "expected 100% v2 after rollout"
    ;;
  blue-green)
    reset_blue
    step "Both pools are ready before switching: blue (v1) and green (v2)"
    "${k[@]}" rollout status deployment/api-green --timeout=120s
    "${k[@]}" get deploy api api-green
    sample 12 0 0 || fail "expected blue before switch"

    jar=$(mktemp)
    trap 'http -b "$jar" -X DELETE "$url/api/me" >/dev/null 2>&1 || true; rm -f "$jar"' EXIT
    tag=$(openssl rand -hex 6)
    email="demo-$tag@example.com"
    password=$(openssl rand -hex 18)
    http -c "$jar" -H 'Content-Type: application/json' \
      -d "{\"email\":\"$email\",\"password\":\"$password\"}" "$url/api/auth/register" >/dev/null
    http -b "$jar" -H 'Content-Type: application/json' \
      -d "{\"url\":\"https://example.com/\",\"alias\":\"demo-$tag\"}" "$url/api/links" >/dev/null

    step "Switch all traffic to green; verify existing cookie and PostgreSQL data on v2"
    route green
    wait_version v2
    sample 12 12 12 || fail "expected green after switch"
    headers=$(http -b "$jar" -D - -o /dev/null "$url/api/me")
    grep -i '^x-shorty-version: v2' <<<"$headers" || fail "the session check did not reach v2"
    http -b "$jar" "$url/api/links" | jq -ce --arg alias "demo-$tag" '.[] | select(.code == $alias)'

    step "Rollback by switching the route back to the still-ready blue pool"
    route blue
    wait_version v1
    sample 12 0 0 || fail "expected blue after rollback"
    "${k[@]}" get deploy api api-green
    ;;
  canary)
    reset_blue
    for weights in 90/10 50/50 0/100; do
      step "Canary weights v1/v2: $weights"
      route "canary-${weights//\//-}"
      case "$weights" in
        90/10) lower=3; upper=28 ;;
        50/50) lower=40; upper=80 ;;
        0/100) lower=120; upper=120; wait_version v2 ;;
      esac
      for attempt in 1 2 3; do
        sleep 2
        if sample 120 "$lower" "$upper"; then break; fi
        (( attempt < 3 )) || fail "observed distribution is inconsistent with $weights"
      done
    done
    ;;
  ab)
    reset_blue
    step "A/B: exact match x-user-group: beta -> v2, default -> v1"
    route ab
    printf 'no header: '; wait_version v1
    printf 'x-user-group: beta: '; wait_version v2 -H 'x-user-group: beta'
    printf 'x-user-group: other: '; wait_version v1 -H 'x-user-group: other'
    ;;
  shadow)
    reset_blue
    step "Shadow: mirror only GET /api/version, full client response from v1"
    route shadow
    tag="shadow-$(openssl rand -hex 6)"
    for ((attempt=1; attempt<=10; attempt++)); do
      id="$tag-$attempt"
      printf 'client (id=%s): ' "$id"
      line=$(response -H "X-Demo-ID: $id")
      echo "$line"
      [[ ${line%% *} == v1 ]] || fail "shadow version answered the client"
      sleep 1
      logs=$("${k[@]}" logs -l 'app.kubernetes.io/name in (api,api-green)' -c api --since=2m --tail=-1 --max-log-requests=10 |
        jq -c --arg id "$id" 'select(.demo_id == $id) | {timestamp, demo_id, version, pod, method, path, status}')
      if jq -se 'length == 2 and (map(.version) | sort) == ["v1","v2"]' <<<"$logs" >/dev/null; then
        printf 'Both pod logs for the same request:\n%s\n' "$logs"
        exit 0
      fi
    done
    fail "matching v1 and v2 log entries were not found"
    ;;
esac
