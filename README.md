# Gemma Atelier

nubia上でBoltとの会話と画像生成を使うためのアプリです。ホーム画面の緑の稲妻の「Gemma」から起動します。会話モデルはNemotron 3.5 Bolt E4B、会話本体はHexagon NPU、画像生成はNightmareのQNN DSPを使います。

## APKを入れる

**v1.1.0は初回セットアップ付きです。対象はSnapdragon 8 Gen 3（SM8650）・RAM16GB・Android12以上。NPUで起動できない端末では停止し、CPU推論へ切り替えません。**

1. AndroidのChromeで[最新Release](https://github.com/RNA4219/gemma-atelier/releases/latest)を開きます。非公開の場合はアクセス可能なGitHubアカウントでログインしてください。
2. `Gemma-Atelier-v1.1.0.apk`をダウンロードして開き、インストールまたは更新を選びます。
3. Gemmaの「初回セットアップ」でTermuxを導入して一度開きます。Androidのインストール確認・権限要求を許可してください。
4. Gemmaに戻り「セットアップをコピーしてTermuxを開く」を押し、Termuxで長押し→貼り付け→Enter。以後、モデル取得・PLE分離・NPU確認を自動で行います。
5. 完了後にGemmaへ戻り「会話を開く」。ホームにGemmaを置けば、次回からワンタップで起動できます。

初回のモデル取得は約6.2GB、空き容量16GiB以上とWi-Fiが必要です。画像生成はNightmareと画像モデルの別設定が必要です。現在のIllustrious・LoRA等を別端末へ自動移行する機能は含みません。[導入手順](docs/SETUP.md)を参照してください。

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
