#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."

profile=${SHORTY_PROFILE:-shorty-rollouts}
mk=(minikube -p "$profile")
k=(kubectl --context "$profile")
ns=("${k[@]}" -n shorty)
eg=("${k[@]}" -n envoy-gateway-system)
gateway_release=https://github.com/envoyproxy/gateway/releases/download/v1.6.4/install.yaml

"${mk[@]}" start --driver=docker --container-runtime=containerd \
  --kubernetes-version=v1.34.0 --cpus=3 --memory=4096

# Build with the host Docker daemon, then load the images into this Minikube profile.
build_ca=()
if [[ -n "${SHORTY_BUILD_CA:-}" ]]; then
  build_ca=(--secret "id=build_ca,src=$SHORTY_BUILD_CA")
fi
for version in v1 v2; do
  docker build "${build_ca[@]}" --build-arg "APP_VERSION=$version" -t "shorty-api:$version" api
  "${mk[@]}" image load --overwrite "shorty-api:$version"
done
docker build -t shorty-web:dev web
"${mk[@]}" image load --overwrite shorty-web:dev

"${k[@]}" apply --server-side -f "$gateway_release"
"${eg[@]}" rollout status deployment/envoy-gateway --timeout=300s
"${k[@]}" apply -f k8s/00-namespace.yaml -f k8s/10-config.yaml -f k8s/11-secrets.yaml -f k8s/20-postgres.yaml
"${ns[@]}" rollout status statefulset/postgres --timeout=300s
"${ns[@]}" delete job migrate --ignore-not-found
"${ns[@]}" set image --local -f k8s/30-migrate.yaml migrate=shorty-api:v1 -o yaml | "${ns[@]}" apply -f -
"${ns[@]}" wait --for=condition=complete job/migrate --timeout=180s

for color in blue green; do
  kubectl kustomize --load-restrictor LoadRestrictionsNone "k8s/strategies/$color" | "${ns[@]}" apply -f -
done
"${ns[@]}" apply -f k8s/50-web.yaml -f k8s/strategies/gateway.yaml
for deploy in api api-green web; do
  "${ns[@]}" rollout status "deployment/$deploy" --timeout=180s
done
"${ns[@]}" wait --for=condition=Programmed gateway/shorty --timeout=180s
"${ns[@]}" get deploy,svc,gateway,httproute
