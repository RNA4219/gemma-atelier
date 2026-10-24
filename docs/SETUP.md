# LLM Atelierの初回セットアップ

## セットアップ済みのnubia

ReleaseからAPKを上書きインストールして、LLM Atelierを押します。端末のURLは`http://127.0.0.1:8085/`。Termuxを手で開く必要はありません。LLM Atelierの通知は、端末内の起動サービスを維持するために表示します。

アプリのパッケージ名は`local.gemma.launcher`です。ホームアイコンを失った場合はアプリ一覧のLLM Atelierをホームへ置けます。

v1.2.1は表示名をLLM Atelierに統一しています。既存の固定ショートカットも同じIDのまま更新します。端末によって旧名が残る場合は一度押してください。更新後の初回起動でランタイムの版を確認し、必要なら既存モデルを再利用して会話UIを更新し、完了後に会話へ戻ります。通常起動のたびにセットアップを行うことはありません。

## 新しい端末へ移す場合

v1.3.0の対象はSnapdragon 8 Gen 3（SM8650 / Hexagon v75）、RAM16GB、Android12以上です。実機確認の対象はnubia NX721J・Android15。SoCが一致してもメーカー側のDSP公開条件によって起動できない場合があるため、全機種対応とはしていません。NPUの43/43層配置・v75ライブラリ・PLE整合性の有効化が確認できなければ停止します。CPUへの代替実行はありません。

初回は空き約14GiB、Wi-Fi、Chromeを用意してください。必要容量は再開時の取得済み量を考慮して再計算します。約6.2GB（5.8GiB）のモデルを取得し、PLE分離・全テンソル照合の成功後に、セットアップ自身が取得した一時原本だけを削除します。重みを再量子化しません。

1. [最新Release](https://github.com/RNA4219/llm-atelier/releases/latest)からAPKを入れます。公開リポジトリのため、GitHubへのログインは不要です。アカウントの認証情報をアプリへ入力する必要はありません。
2. 「はじめる」→「セットアップを始める」。Termuxを取得・SHA256確認後、管理画面を開いていればAndroidのインストールへ進みます。画面を閉じていた場合は戻って「Termuxをインストール」を押します。提供元の許可をオンにして戻るとAndroidのインストール確認へ進みます。公式GitHub版Termux 0.118.3を使います。
3. Termuxを一度開き、初期ファイルの展開が終わるまで待ちます。
4. LLM Atelierに戻り「接続を許可して続ける」。フォルダ選択でTermuxを選び、「このフォルダを使用」→「許可」。Termuxが表示されない場合は左上のメニューから選びます。Downloadや端末ストレージでは接続できません。
5. 既存のTermux設定を保存して、アプリからの起動連携を設定します。AndroidのTermux実行許可を求められたら許可します。必要なパッケージ、同梱NPU実行ファイル、Bolt取得、PLE分離、NPU確認へ自動で進みます。コマンドの入力・貼り付けは不要です。
6. 「ダウンロード」に会話の準備完了が表示されたら「会話を開く」。必要なら「モデル」の「ホーム画面にLLM Atelierを追加」を押します。画像生成は別工程として表示します。

Termuxとの接続を拒否された場合は「接続を設定し直す」から再開できます。キャンセルした許可画面を繰り返し勝手に開くことはありません。Termux設定は変更前に同じフォルダへ`termux.properties.atelier-backup`として保存し、既存の行は保持します。フォルダへの永続アクセス許可は取得しません。連携後はTermux公式のRUN_COMMANDを使用します。

「ダウンロード」で取得量・速度・推定残り時間を確認できます。通信が切れた場合は「続きから再開」。モデル・公式APKとも途中ファイルを再利用します。ハッシュ不一致はエラーにし、「取り直す」でその取得の途中データだけを片付けられます。確認・PLE準備・NPU起動の時間はダウンロード進捗へ混ぜません。この処理中は一時停止ボタンを表示しません。

「ダウンロード設定」でWi-Fiのみの取得、Hugging Face / HF Mirror / HTTPSのカスタム取得元を選べます。Wi-Fi切断時は一時停止し、接続後に再開します。APKの配布元は公式GitHubのままです。いずれも固定SHA256を確認します。ミラーやカスタム取得元の到達性は環境によります。

画面を閉じても前面サービスとTermuxで処理を続けます。OSがアプリを終了した場合は画面を開き直し、保存済み進捗から接続・再開します。Androidの省電力制限による停止まで防ぐ保証はありません。初回環境の準備中はパッケージマネージャーを使うため、モデル取得の一時停止対象には含みません。NPU起動エラーはTermuxの`~/gemma-e4b/bolt-server.log`で確認できます。

通常の会話画面の「モデル・ダウンロード」から管理画面へ戻れます。「モデル」にはBoltと画像エンジンの導入・接続状態を表示します。更新後のnubiaでは「環境を再確認」で既存モデルを再利用し、同梱ランタイムだけ更新できます。

再起動後は最初のロック解除を行い、ホームのLLM Atelierを押します。通常利用でTermuxを手動で開く必要はありません。既存Termux:Boot向けの起動スクリプトも配置しますが、Termux:Boot自体は導入しません。nubiaの省電力管理で停止される場合はLLM AtelierとTermuxの自動起動・バックグラウンド動作を許可してください。

## 画像生成の準備

セットアップ画面から公式Nightmare APKを導入できます。Nightmare側でNPU対応の画像モデルを設定し、APIを有効にして、表示されたAPIトークンをLLM Atelierのセットアップ画面へ貼り付けます。トークンは端末内の認証付きloopback接続でTermuxの非公開ファイルへ渡します。

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

LLM Atelierの前面サービスがTermuxのRunCommandServiceへbindingし、停止中なら固定の`start-from-home.sh`を実行します。会話APIの準備を待ってChromeを開きます。20秒間隔で接続状態を見て、再起動要求には30秒以上間隔を空けます。

Boot入口はBoltのPLEファイルが利用可能になるまで待機します。初回ロック解除前に暗号化された端末内データを読むことはできません。

詳しい外部コマンドの仕様は[Termuxの公式資料](https://github.com/termux/termux-app/wiki/RUN_COMMAND-Intent)を参照してください。

## 検証範囲

新規環境を初期化した実機試験は行わず、既存NX721J上で導入経路を確認します。HTTP再開・ハッシュ不一致・NPU判定は小さなテスト用データで確認します。長時間性能試験、端末再起動の再試験、別メーカーのDSP互換性試験は含みません。
