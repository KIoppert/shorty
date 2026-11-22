# Shared HTTP probes for the demo and traffic panels. The caller sets port, url and k.
http() {
  curl --noproxy '*' --resolve "shorty-rollouts.localhost:$port:127.0.0.1" --max-time 5 -sS --fail-with-body "$@"
}

response() {
  local body
  if ! body=$(http "$@" "$url/api/version"); then
    printf '%s\n' "$body" >&2
    return 1
  fi
  jq -er 'select(.version == "v1" or .version == "v2") | "\(.version)  \(.pod)"' <<<"$body"
}

shadow_proof() {
  local attempts=$1 tag attempt id line logs
  tag="shadow-$(openssl rand -hex 6)" || return 1
  for ((attempt=1; attempt<=attempts; attempt++)); do
    id="$tag-$attempt"
    line=$(response -H "X-Demo-ID: $id") || return 1
    if [[ ${line%% *} != v1 ]]; then
      printf 'Shadow client received %s; expected v1\n' "$line" >&2
      return 1
    fi
    sleep 1
    logs=$("${k[@]}" logs -l 'app.kubernetes.io/name in (api,api-green)' -c api \
      --since=2m --tail=-1 --max-log-requests=10 |
      jq -c --arg id "$id" 'select(.demo_id == $id)') || return 1
    if jq -se 'length == 2 and (map(.version) | sort) == ["v1","v2"]' <<<"$logs" >/dev/null; then
      printf 'Shadow client: %s | demo_id=%s\n' "$line" "$id"
      jq -r '"  pod log: \(.version) \(.pod) | HTTP \(.status) | demo_id=\(.demo_id)"' <<<"$logs"
      return
    fi
  done
  echo "No matching v1 and v2 pod logs for the Shadow request." >&2
  return 1
}
