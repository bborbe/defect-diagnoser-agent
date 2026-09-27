#!/usr/bin/env bash
# Lists vault files matching a glob through the git-rest HTTP API.

if [ "$#" -ne 1 ]; then
  echo "usage: vault-list.sh <glob>" >&2
  exit 2
fi

glob="$1"

if [ -z "${GIT_REST_URL}" ]; then
  echo "GIT_REST_URL is required" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

status="$(curl -sS --globoff --max-time 30 -o "$tmp" -w '%{http_code}' \
  -G --data-urlencode "glob=${glob}" "${GIT_REST_URL}/api/v1/files/")" || {
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
