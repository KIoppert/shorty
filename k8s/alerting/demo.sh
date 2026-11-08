#!/bin/bash
set -u

URL=${URL:-http://shorty.localhost:8080}

case "${1:-}" in
  errors)
    kubectl -n shorty set env deployment/api FAULT_ERROR_RATE=0.3
    echo "api отвечает 500 на 30% запросов, нужна нагрузка: ./k8s/slo/load.sh"
    ;;
  goroutines)
    kubectl -n shorty set env deployment/api FAULT_DELAY=20s
    kubectl -n shorty rollout status deployment/api
    echo "держу 150 зависших запросов 2.5 минуты, запросы идут изнутри кластера прямо в api"
    kubectl -n shorty exec deploy/web -- sh -c '
      end=$(( $(date +%s) + 150 ))
      while [ $(date +%s) -lt $end ]; do
        for i in $(seq 150); do wget -q -T 60 -O /dev/null http://api:8080/nope & done
        wait
      done'
    ;;
  db)
    kubectl -n shorty scale statefulset/postgres --replicas=0
    ;;
  reset)
    kubectl -n shorty set env deployment/api FAULT_ERROR_RATE- FAULT_DELAY-
    kubectl -n shorty scale statefulset/postgres --replicas=1
    ;;
  *)
    echo "использование: $0 errors | goroutines | db | reset"
    exit 1
    ;;
esac
