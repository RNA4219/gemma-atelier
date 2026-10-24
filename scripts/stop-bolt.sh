#!/system/bin/sh
set -eu
ROOT=/data/data/com.termux/files/home/gemma-e4b
if [ ! -f "$ROOT/bolt-server.pid" ]; then exit 0; fi
PID=$(cat "$ROOT/bolt-server.pid")
case "$PID" in ''|*[!0-9]*) exit 1;; esac
EXE=$(readlink "/proc/$PID/exe" 2>/dev/null || true)
case "$EXE" in
    "$ROOT/bin/llama-server-bolt"|"$ROOT/bin/llama-server-bolt (deleted)")
        kill -INT "$PID"
        for TRY in $(seq 1 30); do
            if ! kill -0 "$PID" 2>/dev/null; then break; fi
            sleep 1
        done
        if kill -0 "$PID" 2>/dev/null; then echo 'Bolt is still shutting down'; exit 1; fi;;
    '') ;;
    *) echo 'PID belongs to another process'; exit 1;;
esac
rm "$ROOT/bolt-server.pid"
