#!/usr/bin/env bash
# Reads one vault file through the git-rest HTTP API.

if [ "$#" -ne 1 ]; then
  echo "usage: vault-read.sh <path>" >&2
  exit 2
fi

path="$1"

case "$path" in
  /*)
    echo "invalid path: must not be absolute" >&2
    exit 2
    ;;
esac

case "/$path/" in
  *"/../"*)
    echo "invalid path: must not contain a .. segment" >&2
    exit 2
    ;;
esac

case "$path" in
  *\?* | *"#"* | *"://"*)
    echo "invalid path: must not contain ? # or ://" >&2
    exit 2
    ;;
esac

if [ -z "${GIT_REST_URL}" ]; then
  echo "GIT_REST_URL is required" >&2
  exit 1
fi

encoded="${path//%/%25}"
encoded="${encoded// /%20}"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

status="$(curl -sS --globoff --max-time 30 -o "$tmp" -w '%{http_code}' \
  "${GIT_REST_URL}/api/v1/files/${encoded}")" || {
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
