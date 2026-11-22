#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."

profile=${SHORTY_PROFILE:-minikube}
port=${SHORTY_PORT:-18080}
if [[ ! $port =~ ^[1-9][0-9]{0,4}$ ]] || (( port > 65535 )); then
  echo "SHORTY_PORT must be between 1 and 65535" >&2
  exit 2
fi
mk=(minikube -p "$profile")
k=(kubectl --context "$profile")
ns=("${k[@]}" -n shorty-rollouts)
eg=("${k[@]}" -n envoy-gateway-system)
gateway_release=https://github.com/envoyproxy/gateway/releases/download/v1.10.0/install.yaml

"${mk[@]}" status
"${k[@]}" get nodes

# Reuse the existing workload manifests under a different namespace.
demo_resources() {
  sed -e 's/^  namespace: shorty$/  namespace: shorty-rollouts/' \
    -e "s|^  BASE_URL: http://shorty.localhost:8080$|  BASE_URL: http://shorty-rollouts.localhost:$port|" "$@"
}

# Build with the host Docker daemon, then load the images into this Minikube profile.
build_ca=()
if [[ -n "${SHORTY_BUILD_CA:-}" ]]; then
  build_ca=(--secret "id=build_ca,src=$SHORTY_BUILD_CA")
fi
for version in v1 v2; do
  docker build "${build_ca[@]}" --build-arg "APP_VERSION=$version" -t "shorty-rollout-api:$version" api
  "${mk[@]}" image load --overwrite "shorty-rollout-api:$version"
done
docker build -t shorty-rollout-web:demo web
"${mk[@]}" image load --overwrite shorty-rollout-web:demo

controller=$("${eg[@]}" get deployment envoy-gateway --ignore-not-found -o name)
if [[ -z $controller ]]; then
  "${k[@]}" apply --server-side -f "$gateway_release"
fi
"${eg[@]}" rollout status deployment/envoy-gateway --timeout=300s
"${eg[@]}" get deployment envoy-gateway -o 'jsonpath={.spec.template.spec.containers[*].image}{"\n"}'
"${k[@]}" apply -f k8s/strategies/namespace.yaml
demo_resources k8s/10-config.yaml | "${ns[@]}" apply -f -
demo_resources k8s/11-secrets.yaml | "${ns[@]}" apply -f -
demo_resources k8s/20-postgres.yaml | "${ns[@]}" apply -f -
"${ns[@]}" rollout status statefulset/postgres --timeout=300s
"${ns[@]}" delete job migrate --ignore-not-found
demo_resources k8s/30-migrate.yaml |
  "${ns[@]}" set image --local -f - migrate=shorty-rollout-api:v1 -o yaml |
  "${ns[@]}" apply -f -
"${ns[@]}" wait --for=condition=complete job/migrate --timeout=180s

for color in blue green; do
  kubectl kustomize --load-restrictor LoadRestrictionsNone "k8s/strategies/$color" | "${ns[@]}" apply -f -
done
demo_resources k8s/50-web.yaml | sed 's|image: shorty-web:dev$|image: shorty-rollout-web:demo|' |
  "${ns[@]}" apply -f -
"${k[@]}" apply -f k8s/strategies/gateway.yaml
for deploy in api api-green web; do
  "${ns[@]}" rollout status "deployment/$deploy" --timeout=180s
done
"${ns[@]}" wait --for=condition=Programmed gateway/shorty-rollouts --timeout=180s
"${ns[@]}" get deploy,svc,gateway,httproute
