#!/bin/sh
set -eu

term() {
  echo "[custom-business-metrics] stopping"
  kill -TERM $(jobs -p) 2>/dev/null || true
  wait || true
  exit 0
}

supervise() {
  name="$1"
  shift
  while true; do
    echo "[custom-business-metrics] starting ${name}: $*"
    "$@" &
    pid="$!"
    status=0
    wait "$pid" || status="$?"
    echo "[custom-business-metrics] ${name} stopped with status ${status}; restarting in 2s"
    sleep 2
  done
}

trap term INT TERM

supervise service metrics-service &
supervise agent metrics-agent &
supervise webview busybox httpd -f -p "${METRICS_WEBVIEW_ADDR:-0.0.0.0:5173}" -h /opt/custom-business-metrics/webview &

wait
