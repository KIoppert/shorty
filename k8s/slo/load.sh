#!/bin/bash
set -u

URL=${1:-http://shorty.localhost:8080}
JAR=$(mktemp)
EMAIL="load-$(date +%s)@shorty.dev"
ALIAS="load-$(date +%s)"
hit() { curl -s -o /dev/null -w "%{http_code} " "$@"; }

curl -s -o /dev/null -c "$JAR" "$URL/api/auth/register" -d "{\"email\":\"$EMAIL\",\"password\":\"password123\"}"
curl -s -o /dev/null -b "$JAR" "$URL/api/links" -d "{\"url\":\"https://go.dev\",\"alias\":\"$ALIAS\"}"

echo "нагрузка на $URL, остановить: Ctrl+C"
while true; do
  hit "$URL/$ALIAS"
  hit -b "$JAR" "$URL/api/links"
  hit -b "$JAR" "$URL/api/me"
  hit "$URL/nope"
  echo
  sleep 0.5
done
