'use strict';
function setupStep(s){
 const bolt=s.jobs?.bolt||{},termux=s.jobs?.termux||{};
 if(!s.supported)return {key:'unsupported',title:'この端末は対応待ちです',detail:'現在はSnapdragon 8 Gen 3・RAM 16GB向けです。NPUで動作する対応版が必要です。'};
 if(!s.termux){
  if(termux.state==='running')return {key:'termuxDownload',title:'実行環境をダウンロード中',detail:'取得が終わると、Androidのインストール画面へ進みます。',tab:'downloads',label:'進み具合を見る'};
  return {key:'termux',title:'端末内で話す準備をしましょう',detail:'必要なアプリ、接続の許可、モデル取得の順に案内します。コマンド入力は不要です。',action:'prepare',label:termux.state==='ready'?'Termuxをインストール':termux.state==='paused'||termux.state==='error'?'ダウンロードを再開':'セットアップを始める'};
 }
 if(!s.linked){
  if(!s.termuxOpened)return {key:'openTermux',title:'Termuxを一度開いてください',detail:'最初の展開が終わったら、ここへ戻ります。文字の入力は不要です。',action:'openTermux',label:'Termuxを開く'};
  return {key:'link',title:'Termuxとの接続を許可',detail:'フォルダ選択で「Termux」→「このフォルダを使用」→「許可」。設定を渡したら、モデルの準備へ進みます。',action:'repairLink',label:'接続を許可して続ける'};
 }
 if(bolt.error_code==='termux_link')return {key:'linkError',title:'接続の設定を確認してください',detail:bolt.message||'Termuxの許可を設定し直して再開できます。',action:'repairLink',label:'接続を設定し直す'};
 if(bolt.state==='running')return {key:'running',title:'会話の準備を進めています',detail:bolt.message||'モデルの取得とNPU起動を順に確認します。',tab:'downloads',label:'進み具合を見る'};
 if(bolt.state==='error'||bolt.state==='paused')return {key:'retry',title:bolt.state==='paused'?'準備を一時停止しています':'準備が途中で止まっています',detail:bolt.message,action:'prepare',label:'続きから再開'};
 if(s.installed&&s.runtimeCurrent)return {key:'ready',title:'会話の準備ができました',detail:s.health?.gemma?'端末内のNPUに接続しています。':'モデルは導入済みです。開くと端末内のNPUを起動します。',action:'openChat',label:'会話を開く'};
 return {key:'model',title:s.installed?'アプリの更新を適用します':'会話モデルを準備します',detail:s.installed?'導入済みモデルと会話履歴をそのまま使います。':'約5.8 GiBを取得し、NPUで起動できるか確認します。途中で止まっても再開できます。',action:'prepare',label:s.installed?'更新して続ける':'モデルの準備を始める'};
}
if(typeof module!=='undefined')module.exports={setupStep};
