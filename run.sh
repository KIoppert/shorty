#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

echo "==> minikube"
minikube status >/dev/null 2>&1 || minikube start --driver=docker
minikube addons enable ingress
kubectl -n ingress-nginx rollout status deployment/ingress-nginx-controller --timeout=180s

echo "==> сборка образов внутри minikube"
minikube image build -t shorty-api:dev ./api
minikube image build -t shorty-web:dev ./web
for image in shorty-api:dev shorty-web:dev; do
  minikube image ls | grep -q "$image" || { echo "образ $image не собрался, смотрите вывод сборки выше"; exit 1; }
done

echo "==> деплой"
kubectl delete job migrate -n shorty --ignore-not-found
kubectl apply -f k8s/
kubectl -n shorty rollout restart deployment/api deployment/web deployment/prober

echo "==> ожидание готовности"
kubectl -n shorty rollout status statefulset/postgres --timeout=180s
kubectl -n shorty wait --for=condition=complete job/migrate --timeout=180s
kubectl -n shorty rollout status deployment/api --timeout=180s
kubectl -n shorty rollout status deployment/web --timeout=180s
kubectl -n shorty rollout status deployment/prober --timeout=180s
kubectl -n shorty get pods,svc,ingress

echo
echo "Готово. Доступ к приложению через Ingress-контроллер:"
echo "  kubectl -n ingress-nginx port-forward svc/ingress-nginx-controller 8080:80"
echo "и откройте http://shorty.localhost:8080"
