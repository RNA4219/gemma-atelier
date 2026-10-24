#!/data/data/com.termux/files/usr/bin/sh
set -eu
ROOT=/data/data/com.termux/files/home/gemma-e4b
export PATH=/data/data/com.termux/files/usr/bin:/system/bin
if [ "${1:-}" != '--locked' ]; then
    exec flock -n --close "$ROOT/boot-atelier.lock" "$0" --locked
fi
exec >> "$ROOT/boot-atelier.log" 2>&1
printf '\n[%s] Starting Gemma Atelier after boot\n' "$(date -Iseconds)"
termux-wake-lock
MODEL="$ROOT/models/nemotron-bolt-e4b/ple.gguf"
READY=0
for TRY in $(seq 1 60); do
    if [ -r "$MODEL" ]; then READY=1; break; fi
    sleep 2
done
if [ "$READY" -ne 1 ]; then
    echo 'Model storage is not ready. Unlock the device, then run start-atelier.sh.'
    exit 1
fi
for TRY in 1 2 3; do
    if sh "$ROOT/start-atelier.sh"; then
        printf '[%s] Gemma Atelier is ready\n' "$(date -Iseconds)"
        exit 0
    fi
    printf '[%s] Startup attempt %s failed\n' "$(date -Iseconds)" "$TRY"
    if [ "$TRY" -lt 3 ]; then sleep 10; fi
done
exit 1
