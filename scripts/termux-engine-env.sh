#!/system/bin/sh
GEMMA_ROOT=/data/data/com.termux/files/home/gemma-e4b
export ADSP_LIBRARY_PATH="$GEMMA_ROOT/bin"
export LD_LIBRARY_PATH="/vendor/lib64${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export GGML_HEXAGON_DEVICES=1
export GGML_HEXAGON_ARCH=75
export GGML_HEXAGON_MBUF=512M,256M
export GGML_HEXAGON_VMEM=2G
export GGML_HEXAGON_PROFILE=${GGML_HEXAGON_PROFILE:-0}
GEMMA_MODEL=/sdcard/Download/gemma-e4b/gemma-4-E4B_q4_0-it.gguf
GEMMA_DRAFT=/sdcard/Download/gemma-e4b/gemma-4-E4B-qat-assistant-f16.gguf
