# 開発者向けビルド

## 起動APK

WindowsのPowerShell、JDK17、Android SDK35・Build Tools35.0.0が必要です。

```powershell
pwsh -NoProfile -File scripts/build-launcher.ps1 -JdkRoot 'C:/path/to/jdk17'
```

出力先とSHA256は`launcher/build/latest.json`です。初回に作成する`launcher/private/gemma-local.jks`は、その環境での更新署名に使います。配布済みAPKの署名鍵はGitHubに置いていません。別の鍵でビルドしたAPKは配布済みAPKへ上書きできません。

## 会話ブリッジ

Go 1.26を使用します。GoソースとWeb UI・辞書は`chat-bridge/`内で完結しています。既存のブリッジを更新する場合は端末向けのビルド条件と署名・配備手順を保守環境に合わせてください。

## NPUエンジンのソース

`engine-patches/source.json`に基準commitを固定しています。新しいllama.cppへ無条件に差分を適用しないでください。

```sh
git clone https://github.com/ggml-org/llama.cpp engine
git -C engine checkout d454a204ef2b63af357744809c05af897611a72e
git -C engine apply ../engine-patches/bolt-ple-hexagon.patch
```

Android NDKと別途入手したQualcomm Hexagon SDKが必要です。`scripts/build-android-npu.ps1`へ`-SdkRoot`、`-NdkRoot`、`-Python`を渡します。SDKは再配布していません。今回の配布準備では、NPUエンジンを再ビルドしていません。

## モデル分離

[NVIDIA公式モデル](https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-E4B-4GB)の`Nemotron-3.5-Bolt-E4B-Q4_K_M.gguf`を別途入手します。基準revisionは`4008514238a0b4ae17af3988818f75bf084541a5`、ファイルSHA256は`5442bfb3b1384ff3774c9989e9457dad81bbb5830fbcf96adfdde40078831d5a`です。

`scripts/split_bolt_ple.py --help`で引数を確認してください。`engine/gguf-py`とそのPython依存が必要です。重みを再量子化せず、PLEだけを別GGUFに置きます。元GGUFと分離後の両方を保存できる空き容量が必要です。

実行は`--model-splits core.gguf,ple.gguf --load-mode mmap --lazy-mode on --device HTP0 -ngl 99`を使います。現行の確認済み条件は`GGML_HEXAGON_OPBATCH=1`、`GGML_HEXAGON_OPQUEUE=1`です。通常起動の全条件は`scripts/start-bolt.sh`を正本とします。
