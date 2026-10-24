# 第三者コンポーネント

- llama.cppの差分対象は[ggml-org/llama.cpp](https://github.com/ggml-org/llama.cpp)。元ソースのMIT Licenseは`engine-patches/LLAMA-LICENSE.txt`に収録しています。
- タグ辞書は[DominikDoom/a1111-sd-webui-tagcomplete](https://github.com/DominikDoom/a1111-sd-webui-tagcomplete)から取得。固定commit、元URL、SHA256は`chat-bridge/data/tag-dictionary-source.json`、MIT Licenseは`chat-bridge/data/TAG-DICTIONARY-LICENSE.txt`です。
- Termux、Nightmare、Qualcomm SDK、モデル重みはこのリポジトリの配布物に含めていません。それぞれの配布条件に従って別途入手してください。
- v1.1.0のRelease APKはllama.cppベースのARM64実行ファイル、同ソースからビルドしたHexagon v75プログラム、Goの会話ブリッジを同梱します。ランタイム内の`licenses/`にllama.cppとGoのライセンスを収録しています。Qualcomm SDK自体は含めません。Termux・Nightmareの公式APKとNVIDIAモデルは利用者の操作で各配布元から取得し、ハッシュを照合します。

独自アプリ部分については、この初回登録では新たなオープンソースライセンスを付与していません。
