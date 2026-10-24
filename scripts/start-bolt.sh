#!/system/bin/sh
set -eu
. /data/data/com.termux/files/home/gemma-e4b/engine-env.sh
export PATH=/data/data/com.termux/files/usr/bin:$PATH
export GGML_HEXAGON_PROFILE=0
export GGML_HEXAGON_OPBATCH=1
export GGML_HEXAGON_OPQUEUE=1
if [ "${1:-}" != '--locked' ]; then
    exec flock -n --close "$GEMMA_ROOT/bolt-start.flock" sh "$0" --locked
fi
PID_FILE="$GEMMA_ROOT/bolt-server.pid"
LOG_FILE="$GEMMA_ROOT/bolt-server.log"
MODEL_ROOT="$GEMMA_ROOT/models/nemotron-bolt-e4b"
if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    case "$PID" in ''|*[!0-9]*) echo 'Invalid Bolt PID'; exit 1;; esac
    if kill -0 "$PID" 2>/dev/null; then
        EXE=$(readlink "/proc/$PID/exe" 2>/dev/null || true)
        if [ "$EXE" = "$GEMMA_ROOT/bin/llama-server-bolt" ] && curl -fsS --max-time 2 http://127.0.0.1:8086/health >/dev/null; then exit 0; fi
        echo 'Bolt PID is occupied but not ready; inspect bolt-server.log.'; exit 1
    fi
    rm "$PID_FILE"
fi
if curl -fsS --max-time 1 http://127.0.0.1:8086/health >/dev/null 2>&1; then
    echo 'Port 8086 belongs to another run.'; exit 1
fi
for FILE in core.gguf ple.gguf manifest.json; do
    if [ ! -s "$MODEL_ROOT/$FILE" ]; then echo "Required Bolt file is missing: $FILE"; exit 1; fi
done
nohup "$GEMMA_ROOT/bin/llama-server-bolt" --model-splits "$MODEL_ROOT/core.gguf,$MODEL_ROOT/ple.gguf" \
    --host 127.0.0.1 --port 8086 --parallel 1 --device HTP0 -ngl 99 --fit off \
    -c 8192 -b 128 -ub 32 -t 4 --load-mode mmap --lazy-mode on --no-warmup --log-verbosity 4 \
    --jinja --spec-type none --chat-template-kwargs '{"enable_thinking":false,"conversational_mode":true}' \
    > "$LOG_FILE" 2>&1 < /dev/null &
PID=$!
printf '%s\n' "$PID" > "$PID_FILE"
for TRY in $(seq 1 180); do
    if ! kill -0 "$PID" 2>/dev/null; then break; fi
    if curl -fsS --max-time 1 http://127.0.0.1:8086/health >/dev/null 2>&1; then
        if grep -q 'offloaded 43/43 layers' "$LOG_FILE" && grep -q 'libggml-htp-v75.so' "$LOG_FILE" && grep -q 'PLE integrity enabled' "$LOG_FILE"; then
            echo 'Bolt split PLE / NPU ready: 127.0.0.1:8086'; exit 0
        fi
        break
    fi
    sleep 1
done
kill -INT "$PID" 2>/dev/null || true
echo "Bolt NPU startup failed. Read $LOG_FILE" >&2
exit 1
