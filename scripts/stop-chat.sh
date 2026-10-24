#!/system/bin/sh
set -eu
. /data/data/com.termux/files/home/gemma-e4b/engine-env.sh
PID_FILE="$GEMMA_ROOT/chat-server.pid"
if [ ! -f "$PID_FILE" ]; then
    echo 'No chat PID file.'
    exit 0
fi
CHAT_PID=$(cat "$PID_FILE")
case "$CHAT_PID" in ''|*[!0-9]*) echo 'Invalid chat PID file.' >&2; exit 1;; esac
if kill -0 "$CHAT_PID" 2>/dev/null; then
    if ! tr '\000' '\n' < "/proc/$CHAT_PID/cmdline" | grep -Fxq "$GEMMA_ROOT/bin/llama-server"; then
        echo 'PID belongs to another process; nothing was stopped.' >&2
        exit 1
    fi
    kill -INT "$CHAT_PID"
    for TRY in $(seq 1 20); do
        if ! kill -0 "$CHAT_PID" 2>/dev/null; then break; fi
        sleep 1
    done
    if kill -0 "$CHAT_PID" 2>/dev/null; then
        echo 'Server is still shutting down. Run this script again later.' >&2
        exit 1
    fi
fi
rm -f "$PID_FILE"
echo 'NPU chat stopped.'
