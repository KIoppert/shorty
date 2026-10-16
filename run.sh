#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

echo "==> minikube"
minikube status >/dev/null 2>&1 || minikube start --driver=docker
minikube addons enable ingress
kubectl -n ingress-nginx rollout status deployment/ingress-nginx-controller --timeout=180s

echo "==> сборка образов внутри minikube"
eval "$(minikube docker-env)"
docker build -t shorty-api:dev ./api
docker build -t shorty-web:dev ./web
minikube image load shorty-api:dev
minikube image load shorty-web:dev

echo "==> деплой"
kubectl delete job migrate -n shorty --ignore-not-found
kubectl apply -f k8s/

echo "==> ожидание готовности"
kubectl -n shorty rollout status statefulset/postgres --timeout=180s
kubectl -n shorty wait --for=condition=complete job/migrate --timeout=180s
kubectl -n shorty rollout status deployment/api --timeout=180s
kubectl -n shorty rollout status deployment/web --timeout=180s
kubectl -n shorty get pods,svc,ingress

echo
echo "Готово. Доступ к приложению через Ingress-контроллер:"
echo "  kubectl -n ingress-nginx port-forward svc/ingress-nginx-controller 8080:80"
echo "и откройте http://shorty.localhost:8080"
