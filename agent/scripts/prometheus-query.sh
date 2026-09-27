#!/usr/bin/env bash
# Reads one PromQL instant query through the Prometheus HTTP API.

if [ "$#" -ne 1 ]; then
  echo "usage: prometheus-query.sh <promql>" >&2
  exit 2
fi

expr="$1"

case "$expr" in
  /*)
    echo "invalid query: must not be absolute" >&2
    exit 2
    ;;
esac

case "$expr" in
  *"://"*)
    echo "invalid query: must not contain ://" >&2
    exit 2
    ;;
esac

if [ -z "${PROMETHEUS_URL}" ]; then
  echo "PROMETHEUS_URL is required" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

status="$(curl -sS --globoff --max-time 30 -o "$tmp" -w '%{http_code}' \
  -G --data-urlencode "query=${expr}" "${PROMETHEUS_URL}/api/v1/query")" || {
  echo "request failed" >&2
  exit 1
}

if [ "$status" != "200" ]; then
  echo "$status" >&2
  exit 1
fi

size="$(wc -c < "$tmp")"
size="$((size))"

if [ "$size" -gt 65536 ]; then
  head -c 65536 "$tmp"
  echo truncated >&2
  exit 0
fi

cat "$tmp"
exit 0
