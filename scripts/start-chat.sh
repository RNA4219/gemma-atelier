#!/system/bin/sh
set -eu
. /data/data/com.termux/files/home/gemma-e4b/engine-env.sh
export GGML_HEXAGON_PROFILE=0
PID_FILE="$GEMMA_ROOT/chat-server.pid"
LOG_FILE="$GEMMA_ROOT/chat-server.log"
LOCK_DIR="$GEMMA_ROOT/chat-start.lock"
if ! command -v curl >/dev/null 2>&1; then
    echo 'curl is required. Install it in Termux: pkg install curl' >&2
    exit 1
fi
if [ -f "$PID_FILE" ]; then
    echo 'A chat PID file exists. Run stop-chat.sh first.' >&2
    exit 1
fi
if curl -sS --max-time 1 http://127.0.0.1:8084/health >/dev/null 2>&1; then
    echo 'Port 8084 is already serving a model. Stop that server first.' >&2
    exit 1
fi
if [ ! -f "$GEMMA_ROOT/ui/index.html" ]; then
    echo 'Chat UI is missing: ui/index.html' >&2
    exit 1
fi
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    echo 'Another chat start is in progress.' >&2
    exit 1
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT
nohup "$GEMMA_ROOT/bin/llama-server" -m "$GEMMA_MODEL" \
    --host 127.0.0.1 --port 8084 --parallel 1 \
    --device HTP0 -ngl 99 --fit off -c 2048 -b 32 -ub 32 -t 4 \
    --load-mode mmap --lazy-mode on --no-warmup --log-verbosity 4 \
    --jinja --reasoning off --spec-type none --path "$GEMMA_ROOT/ui" \
    > "$LOG_FILE" 2>&1 < /dev/null &
CHAT_PID=$!
printf '%s\n' "$CHAT_PID" > "$PID_FILE"
READY=0
for TRY in $(seq 1 120); do
    if ! kill -0 "$CHAT_PID" 2>/dev/null; then
        break
    fi
    if curl -fsS --max-time 1 http://127.0.0.1:8084/health >/dev/null 2>&1; then
        if grep -q 'offloaded 43/43 layers' "$LOG_FILE" && grep -q 'libggml-htp-v75.so' "$LOG_FILE"; then
            READY=1
        fi
        break
    fi
    sleep 1
done
if [ "$READY" -ne 1 ]; then
    kill -INT "$CHAT_PID" 2>/dev/null || true
    if ! kill -0 "$CHAT_PID" 2>/dev/null; then rm -f "$PID_FILE"; fi
    echo "NPU startup failed. Read $LOG_FILE" >&2
    exit 1
fi
echo 'NPU chat ready: http://127.0.0.1:8084'
