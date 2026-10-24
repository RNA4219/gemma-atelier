# Gemma Atelier

nubia上でBoltとの会話と画像生成を使うためのアプリです。ホーム画面の緑の稲妻の「Gemma」から起動します。会話モデルはNemotron 3.5 Bolt E4B、会話本体はHexagon NPU、画像生成はNightmareのQNN DSPを使います。

## APKを入れる

**このReleaseは、Termux・Bolt・Nightmareをセットアップ済みの端末向けの起動APKです。APKだけで新しい端末へモデルを導入するものではありません。**

1. AndroidのChromeで[最新Release](https://github.com/RNA4219/gemma-atelier/releases/latest)を開きます。非公開の場合はアクセス可能なGitHubアカウントでログインしてください。
2. `Gemma-Atelier-v1.0.0.apk`をダウンロードして開き、インストールまたは更新を選びます。
3. Androidから求められた場合だけ、Chromeからのアプリインストールと、GemmaからのTermux実行を許可します。
4. ホーム画面またはアプリ一覧の「Gemma」を押します。準備ができると、既存のChromeの会話画面が開きます。

今回セットアップしたnubiaには、このAPKを既にインストールしています。同じ署名による上書き更新に対応します。履歴はChromeの保存領域を使うため、Chromeのデータは消去しないでください。

再起動後は最初のロック解除が必要です。起動サービスが停止していても、アイコンからTermuxとBoltを起動します。現在の検証範囲と必要な環境は[導入・構成](docs/SETUP.md)を参照してください。

## 入っているソース

| フォルダ | 内容 |
|---|---|
| `launcher/` | Androidの起動アプリ。準備待ち、Termuxへの接続維持、Chromeへの遷移 |
| `chat-bridge/` | 会話・画像ツールAPI、キャラクター設定、タグ変換、プリセット、履歴、ノード式ワークフローUI |
| `scripts/` | APKとNPUエンジンのビルド、起動、PLEの物理分離 |
| `engine-patches/` | llama.cppの基準commitと、BoltのPLE別ファイル読み込み・整合性検査の差分 |

モデル・会話履歴・生成画像・認証トークン・署名鍵・Qualcomm SDKは含めていません。画像エンジン本体も別途必要です。ControlNetのUIと接続コードはありますが、実機NPU生成は未完成です。

## 現在の確認範囲

対象はnubia NX721J、Snapdragon 8 Gen 3、Android 15、16GB RAMです。停止したBoltと会話APIをホームアイコンから起動し、Hexagon v75へ43/43層を配置して短い日本語返答を確認しています。既存の会話履歴と画像API接続も確認済みです。今回の起動APKでの端末再起動そのものと長時間常駐の再試験は省略しています。

PLEは専用GGUFから必要な行を読みます。端末内蔵ストレージはUFSであり、NVMeではありません。既存モデルの値を変えずに分離し、起動時にPLE全表をRAMへ読む方式にはしていません。

[ビルド手順](docs/BUILD.md) / [第三者コンポーネント](THIRD_PARTY_NOTICES.md)
