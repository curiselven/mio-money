#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out="${1:-${OUT:-./shots}}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"

chrome="${CHROME:-}"
if [[ -n "$chrome" ]]; then
  if [[ ! -x "$chrome" ]]; then
    echo "CHROME 不可执行：$chrome" >&2
    exit 1
  fi
else
  for candidate in \
    google-chrome \
    chromium \
    chromium-browser \
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    "/Applications/Chromium.app/Contents/MacOS/Chromium"; do
    if [[ -x "$candidate" ]]; then
      chrome="$candidate"
      break
    fi
    if command -v "$candidate" >/dev/null 2>&1; then
      chrome="$(command -v "$candidate")"
      break
    fi
  done
fi
if [[ -z "$chrome" ]]; then
  echo "找不到 Chrome/Chromium；用 CHROME=/path/to/chrome 指定" >&2
  exit 1
fi

site="$("$root/scripts/ui-preview/build.sh")"
shot_timeout="${SHOT_TIMEOUT:-30}"
case "$shot_timeout" in
  ''|*[!0-9]*|0*)
    echo "SHOT_TIMEOUT 必须是正整数：$shot_timeout" >&2
    exit 1
    ;;
esac
extra_chrome_flags="${EXTRA_CHROME_FLAGS:-}"
port="$(python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
)"
server_log="$(mktemp "${TMPDIR:-/tmp}/mio-ui-preview-server.XXXXXX")"

python3 -m http.server "$port" --bind 127.0.0.1 --directory "$site" >"$server_log" 2>&1 &
server_pid=$!

cleanup() {
  kill "$server_pid" 2>/dev/null || true
  wait "$server_pid" 2>/dev/null || true
}
trap cleanup EXIT

ready=""
for _ in {1..50}; do
  if python3 -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:$port/', timeout=0.5)" 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.1
done
if [[ -z "$ready" ]]; then
  echo "本地预览服务启动失败：" >&2
  cat "$server_log" >&2
  exit 1
fi

timeout_cmd=""
if command -v gtimeout >/dev/null 2>&1; then
  timeout_cmd=gtimeout
elif command -v timeout >/dev/null 2>&1; then
  timeout_cmd=timeout
fi

run_with_timeout() {
  RUN_TIMED_OUT=0
  local rc=0
  if [[ -n "$timeout_cmd" ]]; then
    "$timeout_cmd" -k 2 "$shot_timeout" "$@" || rc=$?
    if [[ $rc -eq 124 ]]; then
      RUN_TIMED_OUT=1
    fi
    return "$rc"
  fi

  local flag
  flag="$(mktemp "${TMPDIR:-/tmp}/mio-ui-preview-timeout.XXXXXX")"
  "$@" &
  local pid=$!
  (
    sleep "$shot_timeout"
    echo 1 >"$flag"
    kill -TERM "$pid" 2>/dev/null
    sleep 2
    kill -KILL "$pid" 2>/dev/null
  ) &
  local watcher=$!
  wait "$pid" || rc=$?
  kill "$watcher" 2>/dev/null || true
  wait "$watcher" 2>/dev/null || true
  if [[ -s "$flag" ]]; then
    RUN_TIMED_OUT=1
  fi
  rm -f "$flag"
  return "$rc"
}

failed=""
for page in home import import-done accounts pending; do
  rm -f "$out/$page.png"
  page_rc=0
  run_with_timeout "$chrome" \
    --headless=new \
    --disable-gpu \
    --disable-extensions \
    --hide-scrollbars \
    --no-first-run \
    --no-default-browser-check \
    --window-size=1280,900 \
    --force-device-scale-factor=1 \
    --virtual-time-budget=8000 \
    --run-all-compositor-stages-before-draw \
    $extra_chrome_flags \
    --screenshot="$out/$page.png" \
    "http://127.0.0.1:$port/?page=$page" || page_rc=$?
  if [[ $page_rc -eq 0 && -s "$out/$page.png" ]]; then
    continue
  fi
  if [[ $RUN_TIMED_OUT -eq 1 ]]; then
    echo "截图超时（${shot_timeout}s）：$page" >&2
  elif [[ $page_rc -ne 0 ]]; then
    echo "截图失败（退出码 ${page_rc}）：$page" >&2
  else
    echo "截图失败（未生成文件）：$page" >&2
  fi
  failed="$failed $page"
done

if [[ -n "$failed" ]]; then
  echo "失败页：${failed# }" >&2
  exit 1
fi

printf '截图输出：%s\n' "$out"
