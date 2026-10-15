#!/bin/bash
set -u

URL=${1:-http://localhost:8080}
JAR=$(mktemp)
hit() { curl -s -o /dev/null "$@"; }

echo "регистрация и вход"
hit -c "$JAR" "$URL/api/auth/register" -d '{"email":"demo@shorty.dev","password":"password123"}'
hit -c "$JAR" "$URL/api/auth/login" -d '{"email":"demo@shorty.dev","password":"password123"}'
hit "$URL/api/auth/register" -d '{"email":"demo@shorty.dev","password":"password123"}'
hit "$URL/api/auth/register" -d '{"email":"bob@shorty.dev","password":"short"}'
for i in 1 2 3; do hit "$URL/api/auth/login" -d '{"email":"demo@shorty.dev","password":"wrong-password"}'; done

echo "создание ссылок"
for link in "go-doc https://go.dev/doc/" "k8s https://kubernetes.io/docs/" "habr https://habr.com/ru/articles/" \
            "vlogs https://docs.victoriametrics.com/victorialogs/" "cv-pdf https://example.com/cv.pdf" "rick https://www.youtube.com/watch?v=dQw4w9WgXcQ"; do
  set -- $link
  hit -b "$JAR" "$URL/api/links" -d "{\"url\":\"$2\",\"alias\":\"$1\"}"
done
hit -b "$JAR" "$URL/api/links" -d '{"url":"ftp://bad"}'

echo "переходы по ссылкам"
for pair in go-doc:40 k8s:25 habr:14 vlogs:9 cv-pdf:6 rick:3; do
  for i in $(seq 1 "${pair#*:}"); do
    hit -A "Mozilla/5.0 (iPhone; Mobile)" -e https://t.me/ "$URL/${pair%%:*}"
    hit "$URL/${pair%%:*}"
  done
done
for code in nope missing old-link; do hit "$URL/$code"; done
for i in $(seq 1 15); do hit "$URL/"; hit -b "$JAR" "$URL/api/links"; done

echo "останавливаю postgres на 30 секунд"
kubectl scale deployment/postgres --replicas=0
kubectl wait --for=delete pod -l app=postgres --timeout=60s
for i in $(seq 1 20); do hit "$URL/go-doc"; hit -b "$JAR" "$URL/api/links"; done
sleep 20
kubectl scale deployment/postgres --replicas=1
kubectl rollout status deployment/postgres
until curl -sf "$URL/healthz" >/dev/null; do sleep 2; done

rm -f "$JAR"
echo "готово"
