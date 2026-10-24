# 導入に必要な環境

## セットアップ済みのnubia

ReleaseからAPKを上書きインストールして、Gemmaを押します。端末のURLは`http://127.0.0.1:8085/`。Termuxを手で開く必要はありません。Gemmaの通知は、端末内の起動サービスを維持するために表示します。

アプリのパッケージ名は`local.gemma.launcher`です。ホームアイコンを失った場合はアプリ一覧のGemmaをホームへ置けます。

## 新しい端末へ移す場合

**新端末向けの一括インストーラーは、このReleaseにはありません。** SnapdragonやAndroidの違う端末で動くことも保証していません。以下を先に準備する必要があります。

- 公式Termuxと、必要なら同じ署名系列のTermux:Boot。
- Termuxの`~/.termux/termux.properties`で`allow-external-apps = true`。Android側でGemmaにTermuxのRUN_COMMAND権限を付与。
- Termux内の`curl`、`flock`、Termux:APIアプリを必要としない標準の`termux-wake-lock`。
- 対象Hexagon v75向けにビルドしたBolt対応エンジンとDSPライブラリ。
- 公式Bolt GGUFから分離した`core.gguf`、`ple.gguf`、`manifest.json`。
- 現用NightmareのローカルAPI（8820）と、その端末にインストール済みの画像モデル。認証トークンは各端末内で用意し、リポジトリへ置かない。
- Chrome。既存の保存キーと同じURLで会話UIを開く。

現行の配置は次のとおりです。

```text
~/gemma-e4b/
  bin/
    llama-server-bolt
    libggml-htp-v75.so
    chat-bridge-bolt
  models/nemotron-bolt-e4b/
    core.gguf
    ple.gguf
    manifest.json
  engine-env.sh
  start-atelier.sh
  start-atelier-profile.sh
  start-from-home.sh
  start-bolt.sh
  stop-bolt.sh
  stop-chat.sh
  boot-atelier.sh
  nightmare.token
```

`scripts/termux-engine-env.sh`は端末では`engine-env.sh`という名前で使います。既存の署名やモデル配置と合うように準備し、NPUプロセスは通常のTermuxサービスから起動します。PCの`run-as`から直接NPUプロセスを動かさないでください。

通常設定はBolt、non-thinking、context8192、MTPなし、Gemmaへの自動fallbackなしです。モデルや履歴はこのAPKに内包しません。APKの導入と、上記の推論環境の導入は別です。

## 起動の仕組み

Gemmaの前面サービスがTermuxのRunCommandServiceへbindingし、停止中なら固定の`start-from-home.sh`を実行します。会話APIと画像APIの準備を待ってChromeを開きます。20秒間隔で接続状態を見て、再起動要求には30秒以上間隔を空けます。

Boot入口はBoltのPLEファイルが利用可能になるまで待機します。初回ロック解除前に暗号化された端末内データを読むことはできません。

詳しい外部コマンドの仕様は[Termuxの公式資料](https://github.com/termux/termux-app/wiki/RUN_COMMAND-Intent)を参照してください。
