#!/data/data/com.termux/files/usr/bin/sh
set -eu
ROOT=/data/data/com.termux/files/home/gemma-e4b
export PATH=/data/data/com.termux/files/usr/bin:/system/bin
exec >> "$ROOT/home-start.log" 2>&1
printf '\n[%s] Home icon requested Bolt\n' "$(date -Iseconds)"
termux-wake-lock
exec sh "$ROOT/start-atelier.sh"
