#!/bin/bash
set -u

URL=${1:-http://shorty.localhost:8080}
JAR=$(mktemp)
EMAIL="demo-$(date +%s)@shorty.dev"
hit() { curl -s -o /dev/null "$@"; }

echo "регистрация и вход"
hit -c "$JAR" "$URL/api/auth/register" -d "{\"email\":\"$EMAIL\",\"password\":\"password123\"}"
hit -c "$JAR" "$URL/api/auth/login" -d "{\"email\":\"$EMAIL\",\"password\":\"password123\"}"
hit "$URL/api/auth/register" -d "{\"email\":\"$EMAIL\",\"password\":\"password123\"}"
hit "$URL/api/auth/register" -d '{"email":"bob@shorty.dev","password":"short"}'
for i in 1 2 3; do hit "$URL/api/auth/login" -d "{\"email\":\"$EMAIL\",\"password\":\"wrong-password\"}"; done

echo "создание ссылок"
for link in "go-doc https://go.dev/doc/" "k8s https://kubernetes.io/docs/" "habr https://habr.com/ru/articles/" \
            "vlogs https://docs.victoriametrics.com/victorialogs/" "cv-pdf https://example.com/cv.pdf" "rick https://www.youtube.com/watch?v=dQw4w9WgXcQ"; do
  set -- $link
  hit -b "$JAR" "$URL/api/links" -d "{\"url\":\"$2\",\"alias\":\"$1\"}"
done
hit -b "$JAR" "$URL/api/links" -d '{"url":"ftp://bad"}'
for i in 1 2; do hit -b "$JAR" "$URL/api/links" -d '{"url":"https://example.com/random"}'; done
hit -b "$JAR" "$URL/api/links" -d "{\"url\":\"https://example.com/custom\",\"alias\":\"run-$(date +%s)\"}"
LINK_ID=$(curl -s -b "$JAR" "$URL/api/links" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
for i in 1 2; do hit -b "$JAR" "$URL/api/links/$LINK_ID/qr"; done

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
kubectl -n shorty scale statefulset/postgres --replicas=0
kubectl -n shorty wait --for=delete pod/postgres-0 --timeout=60s
for i in $(seq 1 20); do hit "$URL/go-doc"; hit -b "$JAR" "$URL/api/links"; done
sleep 20
kubectl -n shorty scale statefulset/postgres --replicas=1
kubectl -n shorty rollout status statefulset/postgres
until curl -sf "$URL/healthz" >/dev/null; do sleep 2; done

rm -f "$JAR"
echo "готово"
