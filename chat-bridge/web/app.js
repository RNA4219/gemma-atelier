'use strict';
const $=id=>document.getElementById(id);
const historyKey='gemma-atelier-v1';
let history=[];let busy=false;let connected=false;let imageConnected=false;let tagSource='';
try{const saved=JSON.parse(localStorage.getItem(historyKey)||'[]');if(Array.isArray(saved))history=saved.filter(m=>['user','assistant'].includes(m.role)&&typeof m.content==='string').slice(-40);}catch{}
const savedModel=localStorage.getItem('gemma-atelier-model');if([...$('model').options].some(o=>o.value===savedModel))$('model').value=savedModel;
function persist(){try{localStorage.setItem(historyKey,JSON.stringify(history.slice(-40)));}catch{$('error').textContent='会話履歴を保存できませんでした。'}}
function setBusy(value){busy=value;imageSettingsUI.setBusy(value);$('send').disabled=value||!connected;$('convert-tags').disabled=value||!connected;$('generate-tags').disabled=value||!imageConnected||!$('tags').value.trim();$('tags').disabled=value;$('reset').disabled=value;$('model').disabled=value;document.querySelectorAll('.suggestion').forEach(b=>b.disabled=value);window.dispatchEvent(new CustomEvent('atelier-busy',{detail:value}));}
function add(m){
 const article=document.createElement('article');article.className=m.role;
 const label=document.createElement('strong');label.textContent=m.role==='user'?'あなた':'GEMMA';article.append(label);
 const p=document.createElement('p');p.textContent=m.content;article.append(p);
 if(typeof m.image==='string'&&/^\/images\/[a-f0-9]{24}\.png$/.test(m.image)){
  const image=document.createElement('img');image.src=m.image;image.alt=m.prompt||'生成した画像';image.loading='lazy';article.append(image);
  const save=document.createElement('a');save.href=m.image;save.download='gemma-image.png';save.className='save';save.textContent='画像を保存';article.append(save);
 }
 if(m.prompt){const details=document.createElement('details');const summary=document.createElement('summary');summary.textContent=m.prompt_source==='tags'?'使用したタグ':'変換したプロンプト';const text=document.createElement('p');text.textContent=m.prompt;details.append(summary,text);article.append(details);}
 if(m.image&&typeof m.negative==='string'){const details=document.createElement('details');const summary=document.createElement('summary');summary.textContent='ネガティブ・CFG';const text=document.createElement('p');text.textContent=(m.negative||'指定なし')+'\nCFG '+m.cfg;details.append(summary,text);article.append(details);}
 if(m.seconds){const meta=document.createElement('div');meta.className='meta';meta.textContent=m.image?`${m.model} · 画像 ${m.image_seconds.toFixed(1)}秒 / 全体 ${m.seconds.toFixed(1)}秒`:`${m.seconds.toFixed(1)}秒`;article.append(meta);}
 $('chat').append(article);$('welcome').hidden=true;return article;
}
history.forEach(add);
$('model').addEventListener('change',()=>localStorage.setItem('gemma-atelier-model',$('model').value));
$('reset').addEventListener('click',()=>{history=[];persist();$('chat').replaceChildren();$('welcome').hidden=false;$('error').textContent='';$('prompt').value='';$('tags').value='';tagSource='';$('tag-panel').hidden=true;$('state').textContent='新しい会話 · 端末内で実行';setBusy(false);});
document.querySelectorAll('[data-prompt]').forEach(b=>b.addEventListener('click',()=>{$('prompt').value=b.dataset.prompt;$('prompt').focus();}));
async function connect(){try{const r=await fetch('/api/status',{cache:'no-store'});if(!r.ok)throw Error();const s=await r.json();connected=s.gemma;imageConnected=s.images;$('state').textContent=s.gemma?(s.images?'会話・画像生成に接続済み · 端末内で実行':'会話に接続済み · 画像エンジンは未接続'):'Gemmaを起動してください';if(!s.gemma)$('error').textContent='Termuxで ~/gemma-e4b/start-atelier.sh を実行してください。';}catch{connected=false;imageConnected=false;$('state').textContent='サーバーに接続できません';$('error').textContent='Termuxで ~/gemma-e4b/start-atelier.sh を実行してから再読み込みしてください。';}setBusy(false);}
$('form').addEventListener('submit',async event=>{
 event.preventDefault();const text=$('prompt').value.trim();if(!text||busy||!connected)return;
 const messages=[...history.slice(-29).map(m=>({role:m.role,content:m.content,prompt:m.prompt||'',image:m.image||''})),{role:'user',content:text}];
 await sendRequest('/api/chat',{messages,model:$('model').value},text);
});
async function sendRequest(endpoint,body,text){
 try{body={...body,...imageSettingsUI.current()};}catch(err){$('error').textContent=err.message;return;}
 setBusy(true);$('error').textContent='';const user={role:'user',content:text};const article=add(user);history.push(user);persist();$('prompt').value='';article.scrollIntoView({behavior:'smooth',block:'nearest'});
 let done=false;
 try{
  const response=await fetch(endpoint,{method:'POST',headers:{'Content-Type':'application/json','X-Chat-Client':'1'},body:JSON.stringify(body)});
  if(!response.ok)throw Error(await response.text());
  const reader=response.body.getReader();const decoder=new TextDecoder();let buffer='';
  const onEvent=line=>{
   if(!line.trim())return;const e=JSON.parse(line);
   if(e.type==='error')throw Error(e.message);
   if(e.type==='status')$('state').textContent=e.message;
   if(e.type==='prompt')$('state').textContent='プロンプトを変換しました。画像を生成します…';
   if(e.type==='done'){const reply={role:'assistant',content:e.text,...e};history.push(reply);persist();add(reply).scrollIntoView({behavior:'smooth',block:'nearest'});$('state').textContent=e.trimmed?'完了 · 長い会話の先頭を省いて応答しました':'接続済み · 端末内で実行';if(e.truncated)$('state').textContent='回答の上限に達しました。「続けて」で続きを聞けます。';done=true;}
  };
  while(true){const part=await reader.read();if(part.done)break;buffer+=decoder.decode(part.value,{stream:true});let i;while((i=buffer.indexOf('\n'))>=0){const line=buffer.slice(0,i);buffer=buffer.slice(i+1);onEvent(line);}}
  buffer+=decoder.decode();if(buffer.trim())onEvent(buffer);if(!done)throw Error('生成の途中で接続が切れました。');
 }catch(err){$('error').textContent=err.message;$('state').textContent='生成できませんでした';if(!done){history.pop();persist();article.remove();$('prompt').value=text;}}
 finally{setBusy(false);}
}
$('tags').addEventListener('input',()=>{setBusy(busy);$('tag-note').textContent='編集した内容をそのまま画像生成へ渡します。';});
$('convert-tags').addEventListener('click',async()=>{
 const text=$('prompt').value.trim();if(!text||busy||!connected)return;
 if([...text].length>600){$('error').textContent='タグへの変換は600文字以内で入力してください。';return;}
 setBusy(true);$('error').textContent='';$('state').textContent='Gemmaが生成用タグへ変換しています…';
 try{
  const response=await fetch('/api/tags',{method:'POST',headers:{'Content-Type':'application/json','X-Chat-Client':'1'},body:JSON.stringify({text})});
  if(!response.ok)throw Error(await response.text());const result=await response.json();
  tagSource=text;$('tag-source').textContent='変換元：'+text;$('tags').value=result.prompt;
  $('tag-note').textContent=result.unlisted.length?'辞書にない表現も残しています：'+result.unlisted.join(', '):'タグ辞書の表記に整えました。';
  $('tag-panel').hidden=false;$('tag-panel').scrollIntoView({behavior:'smooth',block:'nearest'});
  $('state').textContent=`${result.tags.length}個のタグに変換 · ${result.seconds.toFixed(1)}秒 · 編集して生成できます`;
 }catch(err){$('error').textContent=err.message;$('state').textContent='タグを変換できませんでした';}
 finally{setBusy(false);}
});
$('generate-tags').addEventListener('click',async()=>{
 const prompt=$('tags').value.trim();if(!prompt||busy||!imageConnected)return;
 await sendRequest('/api/generate',{prompt,model:$('model').value},(tagSource?tagSource+'\n\n':'')+'このタグで生成：'+prompt);
});
connect();
if('serviceWorker' in navigator)navigator.serviceWorker.register('/sw.js').catch(()=>{});
