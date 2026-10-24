# LLM Atelierの初回セットアップ

## セットアップ済みのnubia

ReleaseからAPKを上書きインストールして、Gemmaを押します。端末のURLは`http://127.0.0.1:8085/`。Termuxを手で開く必要はありません。Gemmaの通知は、端末内の起動サービスを維持するために表示します。

アプリのパッケージ名は`local.gemma.launcher`です。ホームアイコンを失った場合はアプリ一覧のGemmaをホームへ置けます。

## 新しい端末へ移す場合

v1.2.0の対象はSnapdragon 8 Gen 3（SM8650 / Hexagon v75）、RAM16GB、Android12以上です。実機確認の対象はnubia NX721J・Android15。SoCが一致してもメーカー側のDSP公開条件によって起動できない場合があるため、全機種対応とはしていません。NPUの43/43層配置・v75ライブラリ・PLE整合性の有効化が確認できなければ停止します。CPUへの代替実行はありません。

初回は空き約14GiB、Wi-Fi、Chromeを用意してください。必要容量は再開時の取得済み量を考慮して再計算します。約6.2GB（5.8GiB）のモデルを取得し、PLE分離・全テンソル照合の成功後に、セットアップ自身が取得した一時原本だけを削除します。重みを再量子化しません。

1. [最新Release](https://github.com/RNA4219/llm-atelier/releases/latest)からAPKを入れます。公開リポジトリのため、GitHubへのログインは不要です。アカウントの認証情報をアプリへ入力する必要はありません。
2. 「はじめる」→「Termuxを準備する」。「ダウンロード」で取得・SHA256確認後、「インストール」を押します。提供元の許可をオンにして戻るとAndroidのインストール確認へ進みます。公式GitHub版Termux 0.118.3を使います。
3. Termuxを一度開き、初期ファイルの展開が終わるまで待ちます。
4. Gemmaに戻り「モデルの準備をはじめる」。実行許可を求められたら許可します。許可後は操作を継続します。
5. Termuxの入力欄を長押しして貼り付け、Enterを押します。必要なパッケージ、同梱NPU実行ファイル、Boltの取得、PLE分離、起動確認が進みます。初回コマンドの貼り付けはAndroidのアプリ分離に対応する利用者操作です。
6. 「ダウンロード」に「準備できました」と表示されたら「会話を開く」。必要なら「モデル」の「ホーム画面にGemmaを追加」を押します。

「ダウンロード」で取得量・速度・推定残り時間を確認できます。通信が切れた場合は「続きから再開」。モデル・公式APKとも途中ファイルを再利用します。ハッシュ不一致はエラーにし、「取り直す」でその取得の途中データだけを片付けられます。確認・PLE準備・NPU起動の時間はダウンロード進捗へ混ぜません。この処理中は一時停止ボタンを表示しません。

「ダウンロード設定」でWi-Fiのみの取得、Hugging Face / HF Mirror / HTTPSのカスタム取得元を選べます。Wi-Fi切断時は一時停止し、接続後に再開します。APKの配布元は公式GitHubのままです。いずれも固定SHA256を確認します。ミラーやカスタム取得元の到達性は環境によります。

画面を閉じても前面サービスとTermuxで処理を続けます。OSがアプリを終了した場合は画面を開き直し、保存済み進捗から接続・再開します。Androidの省電力制限による停止まで防ぐ保証はありません。初回環境の準備中はパッケージマネージャーを使うため、モデル取得の一時停止対象には含みません。NPU起動エラーはTermuxの`~/gemma-e4b/bolt-server.log`で確認できます。

通常の会話画面の「モデル・ダウンロード」から管理画面へ戻れます。「モデル」にはBoltと画像エンジンの導入・接続状態を表示します。更新後のnubiaでは「環境を再確認」で既存モデルを再利用し、同梱ランタイムだけ更新できます。

再起動後は最初のロック解除を行い、ホームのGemmaを押します。通常利用でTermuxを手動で開く必要はありません。既存Termux:Boot向けの起動スクリプトも配置しますが、Termux:Boot自体は導入しません。nubiaの省電力管理で停止される場合はGemmaとTermuxの自動起動・バックグラウンド動作を許可してください。

## 画像生成の準備

セットアップ画面から公式Nightmare APKを導入できます。Nightmare側でNPU対応の画像モデルを設定し、APIを有効にして、表示されたAPIトークンをGemmaのセットアップ画面へ貼り付けます。トークンは端末内の認証付きloopback接続でTermuxの非公開ファイルへ渡します。

会話は画像エンジン未設定でも利用できます。既存nubiaの更新ではNightmareのモデル・APIトークン・画像・会話履歴を消去しません。**新規端末へのIllustrious DMD2・CyberRealistic DMD2・LoRAの自動取得とモデル一覧の自動調整は、この版に含まれません。** 既定UIの画像モデル名とNightmare側の導入モデルが一致する必要があります。画像生成の成功は、会話NPUセットアップの完了条件とは別です。

## 配置

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

通常設定はBolt、non-thinking、context8192、MTPなし、Gemmaへの自動fallbackなしです。モデルや履歴はAPKに内包しません。NPUランタイムをAPKから渡し、モデルは公式配布元の固定revisionからSHA256照合付きで取得します。PLEは内蔵UFS上の専用GGUFから必要な行を読む方式です。

## 起動の仕組み

Gemmaの前面サービスがTermuxのRunCommandServiceへbindingし、停止中なら固定の`start-from-home.sh`を実行します。会話APIの準備を待ってChromeを開きます。20秒間隔で接続状態を見て、再起動要求には30秒以上間隔を空けます。

Boot入口はBoltのPLEファイルが利用可能になるまで待機します。初回ロック解除前に暗号化された端末内データを読むことはできません。

詳しい外部コマンドの仕様は[Termuxの公式資料](https://github.com/termux/termux-app/wiki/RUN_COMMAND-Intent)を参照してください。

## 検証範囲

新規環境を初期化した実機試験は行わず、既存NX721J上で導入経路を確認します。HTTP再開・ハッシュ不一致・NPU判定は小さなテスト用データで確認します。長時間性能試験、端末再起動の再試験、別メーカーのDSP互換性試験は含みません。
