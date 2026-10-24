# Gemma Atelier

nubia上でBoltとの会話と画像生成を使うためのアプリです。ホーム画面の緑の稲妻の「Gemma」から起動します。会話モデルはNemotron 3.5 Bolt E4B、会話本体はHexagon NPU、画像生成はNightmareのQNN DSPを使います。

## APKを入れる

**v1.2.0は導入ガイド・ダウンロード管理付きです。対象はSnapdragon 8 Gen 3（SM8650）・RAM16GB・Android12以上。NPUで起動できない端末では停止し、CPU推論へ切り替えません。**

1. AndroidのChromeで[最新Release](https://github.com/RNA4219/gemma-atelier/releases/latest)を開きます。非公開の場合はアクセス可能なGitHubアカウントでログインしてください。
2. `Gemma-Atelier-v1.2.0.apk`をダウンロードして開き、インストールまたは更新を選びます。
3. 「はじめる」の案内に沿ってTermuxを導入し、一度開きます。Androidのインストール確認・権限要求を許可してください。
4. 「モデルの準備をはじめる」から、初回だけTermuxで長押し→貼り付け→Enter。以後、モデル取得・PLE分離・NPU確認を自動で行います。
5. 「ダウンロード」で進み具合を確認し、完了後に「会話を開く」。会話画面の「モデル・ダウンロード」から管理画面へ戻れます。

初回のモデル取得は約6.2GB（5.8GiB）、空き容量約14GiBが目安です。取得量・速度・残り時間を表示し、一時停止と再開に対応します。Wi-Fiのみの取得やミラーも選べます。画像生成はNightmare側で画像モデルを設定します。現在のIllustrious・LoRA等の別端末への自動移行は含みません。[導入手順](docs/SETUP.md)を参照してください。

今回セットアップしたnubiaには、このAPKを既にインストールしています。同じ署名による上書き更新に対応します。履歴はChromeの保存領域を使うため、Chromeのデータは消去しないでください。

再起動後は最初のロック解除が必要です。起動サービスが停止していても、アイコンからTermuxとBoltを起動します。現在の検証範囲と必要な環境は[導入・構成](docs/SETUP.md)を参照してください。

## 入っているソース

| フォルダ | 内容 |
|---|---|
| `launcher/` | Androidの起動・初回導入アプリ。公式APK取得、Termuxへの受け渡し、接続維持 |
| `setup/` | モデルの再開可能な取得・SHA256照合・PLE分離・NPU確認 |
| `chat-bridge/` | 会話・画像ツールAPI、キャラクター設定、タグ変換、プリセット、履歴、ノード式ワークフローUI |
| `scripts/` | APKとNPUエンジンのビルド、起動、PLEの物理分離 |
| `engine-patches/` | llama.cppの基準commitと、BoltのPLE別ファイル読み込み・整合性検査の差分 |

モデル・会話履歴・生成画像・認証トークン・署名鍵・Qualcomm SDKは含めていません。Release APKには確認済みARM64実行ファイルとHexagon v75プログラムを同梱し、モデルとTermuxは公式配布元から取得します。ControlNetのUIと接続コードはありますが、実機NPU生成は未完成です。

## 現在の確認範囲

対象はnubia NX721J、Snapdragon 8 Gen 3、Android 15、16GB RAMです。停止したBoltと会話APIをホームアイコンから起動し、Hexagon v75へ43/43層を配置して短い日本語返答を確認しています。既存の会話履歴と画像API接続も確認済みです。今回の起動APKでの端末再起動そのものと長時間常駐の再試験は省略しています。

PLEは専用GGUFから必要な行を読みます。端末内蔵ストレージはUFSであり、NVMeではありません。既存モデルの値を変えずに分離し、起動時にPLE全表をRAMへ読む方式にはしていません。

[ビルド手順](docs/BUILD.md) / [第三者コンポーネント](THIRD_PARTY_NOTICES.md)
