#!/data/data/com.termux/files/usr/bin/sh
set -eu
ROOT=/data/data/com.termux/files/home/gemma-e4b
export PATH=/data/data/com.termux/files/usr/bin:/system/bin
if [ -f "$ROOT/atelier.pid" ]; then
    PID=$(cat "$ROOT/atelier.pid")
    case "$PID" in ''|*[!0-9]*) echo 'Invalid bridge PID'; exit 1;; esac
    if kill -0 "$PID" 2>/dev/null; then
        EXE=$(readlink "/proc/$PID/exe" || true)
        case "$EXE" in "$ROOT/bin/chat-bridge-bolt"|"$ROOT/bin/chat-bridge-bolt (deleted)") kill -TERM "$PID";; *) echo 'Bridge PID does not belong to Bolt'; exit 1;; esac
        for TRY in 1 2 3 4 5; do
            if ! kill -0 "$PID" 2>/dev/null; then break; fi
            sleep 1
        done
        if kill -0 "$PID" 2>/dev/null; then echo 'Bridge is still stopping'; exit 1; fi
    fi
    rm "$ROOT/atelier.pid"
fi
exec sh "$ROOT/start-atelier.sh"
