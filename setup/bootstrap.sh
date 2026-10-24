#!/data/data/com.termux/files/usr/bin/bash
set -eu
umask 077
termux-wake-lock
mkdir -p "$HOME/gemma-e4b"
printf '%s' "$SETUP_URL" > "$HOME/gemma-e4b/.setup-callback.$$.new"
mv "$HOME/gemma-e4b/.setup-callback.$$.new" "$HOME/gemma-e4b/.setup-callback"
if command -v python >/dev/null && [ -f "$HOME/gemma-e4b/.setup-runtime/download.py" ]; then
    if python "$HOME/gemma-e4b/.setup-runtime/install.py" --callback "$SETUP_URL" --attach-only; then exit 0; fi
fi
printf '\nLLM Atelier: Bolt / Hexagon NPU セットアップ\n'
curl -fsS --max-time 5 -H 'Content-Type: application/json' -d '{"state":"running","phase":"prerequisites","message":"必要なアプリの中身を準備しています。初回は数分かかることがあります。"}' "$SETUP_URL/progress" >/dev/null || true
trap 'CODE=$?; if [ "$CODE" -ne 0 ]; then curl -fsS --max-time 5 -H "Content-Type: application/json" -d "{\"state\":\"error\",\"message\":\"Termuxの準備で停止しました。Termuxのエラー表示と通信を確認して、セットアップを再試行してください。\"}" "$SETUP_URL/progress" >/dev/null || true; fi' EXIT
if ! command -v python >/dev/null || ! command -v flock >/dev/null; then
    pkg update -y
    DEBIAN_FRONTEND=noninteractive pkg install -y -o Dpkg::Options::=--force-confold python curl coreutils util-linux
fi
mkdir -p "$HOME/gemma-e4b/.setup-runtime"
if [ "${1:-}" != '--locked' ]; then
    exec flock -n --close "$HOME/gemma-e4b/.setup-bootstrap.lock" bash "$0" --locked
fi
cd "$HOME/gemma-e4b/.setup-runtime"
curl -f --retry 3 --connect-timeout 10 "$SETUP_URL/runtime.tar" -o runtime.tar.new
printf '%s  %s\n' "$RUNTIME_SHA" runtime.tar.new | sha256sum -c -
mv runtime.tar.new runtime.tar
tar -xf runtime.tar
exec python install.py --callback "$SETUP_URL"
