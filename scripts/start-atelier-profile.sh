#!/system/bin/sh
set -eu
ROOT=/data/data/com.termux/files/home/gemma-e4b
export PATH=/data/data/com.termux/files/usr/bin:$PATH
if [ "${1:-}" != '--locked' ]; then
    exec flock -n --close "$ROOT/atelier-profile.flock" sh "$0" --locked
fi
ATELIER_LLM_PROFILE=bolt
ATELIER_LLM_THINKING=off
ATELIER_LLM_FALLBACK=none
ATELIER_IMAGES_OPTIONAL=0
if [ -f "$ROOT/atelier-llm.env" ]; then . "$ROOT/atelier-llm.env"; fi
case "$ATELIER_LLM_PROFILE" in gemma|bolt) ;; *) echo 'Invalid LLM profile'; exit 1;; esac
case "$ATELIER_LLM_THINKING" in on|off) ;; *) echo 'Invalid thinking setting'; exit 1;; esac
case "$ATELIER_LLM_FALLBACK" in none|gemma) ;; *) echo 'Invalid fallback setting'; exit 1;; esac
termux-wake-lock || true
if [ "$ATELIER_LLM_PROFILE" = bolt ]; then
    sh "$ROOT/stop-chat.sh"
    if ! sh "$ROOT/start-bolt.sh"; then
        if [ "$ATELIER_LLM_FALLBACK" = gemma ]; then
            echo 'Bolt failed; configured Gemma rollback is active.'
            ATELIER_LLM_PROFILE=gemma
        else
            echo 'PLE/Bolt NPU load failed. Read bolt-server.log.'
            exit 1
        fi
    fi
fi
if [ "$ATELIER_LLM_PROFILE" = gemma ]; then
    sh "$ROOT/stop-bolt.sh"
    if ! curl -fsS --max-time 2 http://127.0.0.1:8084/health >/dev/null 2>&1; then
        if [ -f "$ROOT/chat-server.pid" ]; then
            PID=$(cat "$ROOT/chat-server.pid")
            case "$PID" in ''|*[!0-9]*) exit 1;; esac
            if ! kill -0 "$PID" 2>/dev/null; then rm "$ROOT/chat-server.pid"; fi
        fi
        sh "$ROOT/start-chat.sh"
    fi
    ATELIER_LLM_CONTEXT=2048
    LLM=http://127.0.0.1:8084
    BRIDGE="$ROOT/bin/chat-bridge"
else
    ATELIER_LLM_CONTEXT=8192
    LLM=http://127.0.0.1:8086
    BRIDGE="$ROOT/bin/chat-bridge-bolt"
fi
export ATELIER_LLM_PROFILE ATELIER_LLM_CONTEXT ATELIER_LLM_THINKING
# Preserve the image engine used by the deployed application before this change.
if [ -s "$ROOT/nightmare.token" ] && ! curl -fsS --max-time 2 http://127.0.0.1:8820/info >/dev/null 2>&1; then
    /system/bin/am start-foreground-service --user 0 -n com.abrah.nightmare/.OpService --es op api --es arg on || true
    for TRY in $(seq 1 15); do
        if curl -fsS --max-time 2 http://127.0.0.1:8820/info >/dev/null 2>&1; then break; fi
        sleep 1
    done
fi
IMAGE_FLAG=
if [ "$ATELIER_IMAGES_OPTIONAL" = 1 ]; then
    IMAGE_FLAG=-allow-missing-images
elif ! curl -fsS --max-time 2 http://127.0.0.1:8820/info >/dev/null 2>&1; then
    echo 'Nightmare local API unavailable'; exit 1
fi
if ! curl -fsS --max-time 2 http://127.0.0.1:8085/api/status >/dev/null 2>&1; then
    nohup "$BRIDGE" $IMAGE_FLAG -llm "$LLM" -data "$ROOT/chat-data" -nightmare http://127.0.0.1:8820 -token-file "$ROOT/nightmare.token" > "$ROOT/atelier.log" 2>&1 < /dev/null &
    printf '%s\n' "$!" > "$ROOT/atelier.pid"
    printf '%s\n' "$ATELIER_LLM_PROFILE" > "$ROOT/atelier-llm-active"
    sleep 1
fi
curl -fsS --max-time 5 http://127.0.0.1:8085/api/status >/dev/null
echo "Atelier $ATELIER_LLM_PROFILE: http://127.0.0.1:8085"
