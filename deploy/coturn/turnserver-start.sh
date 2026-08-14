#!/bin/sh
set -eu

SECRET_FILE="${COTURN_REST_SECRET_FILE:-/run/secrets/coturn-rest-secret}"
CONF="${COTURN_CONFIG:-/etc/raypx2/turnserver.conf}"
TURNSERVER_BIN="${TURNSERVER_BIN:-/usr/local/bin/turnserver}"

if [ ! -f "$SECRET_FILE" ]; then
  echo "coturn secret file missing: $SECRET_FILE" >&2
  exit 1
fi

MODE=$(stat -c '%a' "$SECRET_FILE")
OTHERS="${MODE#"${MODE%?}"}"
if [ "$OTHERS" -ge 4 ]; then
  echo "coturn secret file is world-readable: $SECRET_FILE mode $MODE" >&2
  exit 1
fi

if [ ! -f "$CONF" ]; then
  echo "coturn config missing: $CONF" >&2
  exit 1
fi

SECRET=$(tr -d '\n' < "$SECRET_FILE")
if [ -z "$SECRET" ]; then
  echo "coturn secret file is empty: $SECRET_FILE" >&2
  exit 1
fi

exec "$TURNSERVER_BIN" -c "$CONF" --static-auth-secret="$SECRET"
