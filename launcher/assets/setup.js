'use strict';
const $=id=>document.getElementById(id);
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const size=n=>n>=1073741824?(n/1073741824).toFixed(1)+' GiB':n>=1048576?(n/1048576).toFixed(1)+' MiB':Math.max(0,n/1024).toFixed(0)+' KiB';
const eta=n=>n<0?'残り時間を計算中':n<60?'残り 約1分':n<3600?'残り 約'+Math.ceil(n/60)+'分':'残り 約'+(n/3600).toFixed(1)+'時間';
let state=null,settingsLoaded=false,toastTimer;
function toast(text){$('toast').textContent=text;$('toast').hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>$('toast').hidden=true,4200);}
function call(action,data={}){if(window.Atelier)Atelier.action(action,JSON.stringify(data));else toast('Androidアプリで利用できます');}
function tab(name){document.querySelectorAll('.view').forEach(v=>v.hidden=v.id!==name);document.querySelectorAll('[data-tab]').forEach(b=>{b.classList.toggle('selected',b.dataset.tab===name);if(b.dataset.tab===name)b.setAttribute('aria-current','page');else b.removeAttribute('aria-current');});window.scrollTo(0,0);}
function html(id,value){if($(id).innerHTML!==value)$(id).innerHTML=value;}
const button=(text,action,id='bolt',cls='primary')=>`<button class="${cls}" data-action="${action}" data-id="${id}">${text}</button>`;
const phaseLabel={connection:'Termuxへ接続中',prerequisites:'環境を準備中',runtime:'準備中',download:'ダウンロード中',waiting:'再接続中',verify:'ファイルを確認中',split:'PLEを準備中',npu:'NPUを起動中',ready:'準備完了'};
const names={bolt:'Nemotron Bolt',termux:'Termux',nightmare:'Nightmare'};
function job(id){const j={...(state.jobs?.[id]||{})};if((id==='bolt'&&state.installed&&!j.state)||(id==='termux'&&state.termux)||(id==='nightmare'&&state.nightmare))Object.assign(j,{state:'complete',phase:'ready',message:id==='bolt'?'この端末に導入済みです':'インストール済みです'});return j;}
function downloadCard(id){
 const j=job(id),running=j.state==='running',complete=j.state==='complete',ready=j.state==='ready',error=j.state==='error',paused=j.state==='paused',total=j.total||0,n=j.bytes||0;
 const label=complete?'導入済み':ready?'インストール可':paused?'一時停止':error?'要確認':running?(phaseLabel[j.phase]||'準備中'):'未取得';
 const description=id==='bolt'?'会話モデル · E4B / Q4_K_M':id==='termux'?'端末内の実行環境 · 公式APK':'画像生成エンジン · 公式APK';
 let actions='';
 if(running){if(['download','waiting'].includes(j.phase))actions=button('一時停止','pause',id,'secondary');}
 else if(complete)actions=id==='bolt'?button('会話を開く ↗','openChat'):button('開く','start',id,'secondary');
 else if(id==='bolt'&&(!state.linked||j.error_code==='termux_link'))actions=button('接続を設定する','prepare');
 else actions=button(ready?'インストール':paused||error?'続きから再開':'ダウンロード',id==='bolt'?'prepare':'start',id)+(paused||error?button('取り直す','reset',id,'text-button'):'');
 const stale=running&&j.updated_at&&Date.now()/1000-j.updated_at>35;
 if(stale)actions=button('接続し直す','start',id,'secondary');
 return `<article class="panel"><div class="card-top"><div class="model-icon ${id==='nightmare'?'image':''}" aria-hidden="true">${id==='bolt'?'ϟ':id==='termux'?'›_':'✧'}</div><div><h2>${names[id]}</h2><p>${description}</p></div><span class="pill ${complete||ready?'good':error||paused?'warn':''}">${label}</span></div>
 <p class="job-message ${error?'error-box':''}">${esc(stale?'進捗への接続が途切れています。データを保持して接続し直せます。':j.message||(id==='bolt'?'約5.8 GiBを取得します。導入後はオフラインで会話できます。':'公式配布ファイルを取得し、SHA-256を確認します。'))}</p>
 ${!complete&&!ready&&(running||paused||error)?`<progress class="progress" aria-label="${esc(names[id])}の${phaseLabel[j.phase]||'進捗'}" max="${total||100}" ${total>0?`value="${Math.min(n,total)}"`:''}></progress><div class="stats"><span>${total>0?`${size(n)} / ${size(total)}`:'この工程の完了を待っています'}</span><span>${j.speed>0?size(j.speed)+'/秒 · '+eta(j.eta??-1):running&&j.phase==='download'?'速度を計算中':''}</span></div>`:''}
 ${id==='bolt'&&!complete?`<div class="phase-steps">${['download','verify','split','npu'].map((p,i)=>`<span class="${j.phase===p?'active':''}">${['取得','確認','PLE準備','NPU起動'][i]}</span>`).join('')}</div>`:''}
 ${actions?`<div class="actions">${actions}</div>`:''}</article>`;
}
function render(s){
 state=s;const bolt=job('bolt'),running=bolt.state==='running',ready=s.installed&&s.runtimeCurrent,health=s.health||{},step=setupStep(s);
 $('network').textContent=s.wifi?'● Wi-Fi':'Wi-Fi未接続';$('network').className='pill '+(s.wifi?'good':'warn');
 const action=step.action?button(esc(step.label),'prepare'===step.action?'prepare':step.action):step.tab?`<button class="primary" data-tab="${step.tab}">${esc(step.label)}</button>`:'<button disabled>NPU対応版が必要です</button>';
 html('welcome',`<div class="hero"><span class="eyebrow">LLM ATELIER SETUP</span><h1>${esc(step.title)}</h1><p>${esc(step.detail)}</p>${action}</div>`);
 const steps=[['Termuxをインストール',s.termux?'インストール済み':'公式APKを取得し、Androidの確認画面で許可',s.termux],['Termuxとの接続を許可',s.linked?'接続設定済み。コマンド入力は不要です':'一度開いてから、Termuxフォルダへのアクセスを許可',s.linked],['Boltを取得・NPUで確認',ready?'モデルとNPUの初回確認が完了しています':running?(bolt.message||'準備中です'):'約5.8 GiBを取得。中断しても続きから再開',ready]];
 let first=true;html('guide',`<div class="section-title"><h2>会話のセットアップ</h2><span>${steps.filter(x=>x[2]).length} / 3</span></div><div class="panel">${steps.map(([title,note,done],i)=>{const active=!done&&first;if(active)first=false;return `<div class="step ${done?'done':active?'current':''}"><span class="step-number">${done?'✓':i+1}</span><div><b>${title}</b><p>${esc(note)}</p></div></div>`;}).join('')}</div><div class="panel"><div class="card-top"><div><h2>画像生成の準備</h2><p>会話のセットアップとは別の工程です</p></div><span class="pill ${health.images?'good':'warn'}">${health.images?'API接続済み':'未完了'}</span></div><p class="job-message">${health.images?'画像APIに接続しています。利用できる画像モデルはNightmare側で確認できます。':'画像モデルの導入とAPIの接続設定が必要です。会話だけ先に使えます。'}</p><button class="secondary" data-tab="models">画像の準備を確認</button></div>`);
 html('device',`<div class="panel"><div class="device-line"><strong>${esc(s.device)}</strong><span>${esc(s.soc)} · RAM ${(s.ram/1073741824).toFixed(0)} GiB</span></div><div class="device-line"><strong>空き容量 ${size(s.free)}</strong><span class="pill ${s.supported?'good':'warn'}">${s.supported?'NPU対応':'未対応'}</span></div><p class="hint">${ready?'導入済みのモデルを再利用します。会話履歴やキャラクター設定もそのまま。':'初回は空き約14 GiBが目安です。取得データと分離後のモデルを一時的に両方保存し、完了後に元データを片付けます。'}</p></div>`);
 html('jobs',['bolt','termux','nightmare'].map(downloadCard).join(''));
 const images=!!health.images;
 html('collection',`<article class="panel"><div class="card-top"><div class="model-icon">ϟ</div><div><h2>Nemotron Bolt</h2><p>NVIDIA · 会話 / E4B</p></div><span class="pill ${ready?'good':''}">${ready?'導入済み':s.installed?'更新待ち':'未導入'}</span></div><p class="job-message">会話、キャラクター、画像生成のプロンプトづくり。普段使いの会話モデルです。</p><div class="tags"><span>NPU推論</span><span>PLE分離</span><span>端末内で処理</span></div><div class="actions">${ready?button('会話を開く ↗','openChat'):s.installed?button('更新して続ける','prepare'):'<button class="primary" data-tab="home">セットアップへ</button>'}${ready?button('環境を再確認','start','bolt','text-button'):''}</div></article>
 <article class="panel"><div class="card-top"><div class="model-icon image">✧</div><div><h2>画像のアトリエ</h2><p>Nightmare · NPUエンジン</p></div><span class="pill ${images?'good':'warn'}">${images?'API接続済み':'準備が必要'}</span></div><p class="job-message">画像モデルの取得・APIの有効化はNightmare側で行います。APKを入れただけでは画像生成の準備は完了しません。</p><ol class="instructions"><li>${s.nightmare?'✓ エンジン導入済み':'画像エンジンを導入'}</li><li>NightmareでNPU対応の画像モデルを取得</li><li>${images?'✓ API接続済み':'下の接続設定からAPIトークンを設定'}</li></ol><div class="actions">${button(s.nightmare?'Nightmareを開く':'画像エンジンを追加','start','nightmare','secondary')}${images?button('画像づくりへ ↗','openChat'):''}</div></article>`);
 $('download-dot').hidden=!Object.values(s.jobs||{}).some(j=>j.state==='running');
 if(!settingsLoaded){$('wifi-only').checked=s.wifiOnly;$('source').value=['https://huggingface.co','https://hf-mirror.com'].includes(s.origin)?s.origin:'custom';$('custom-source').value=s.origin;sourceChange();settingsLoaded=true;}
}
function sourceChange(){const custom=$('source').value==='custom';$('custom-source').hidden=!custom;$('custom-label').hidden=!custom;}
document.addEventListener('click',event=>{const b=event.target.closest('button');if(!b||b.disabled)return;if(b.dataset.tab)tab(b.dataset.tab);if(b.dataset.action){if(b.dataset.action==='start'&&(b.dataset.id==='bolt'||!state?.[b.dataset.id]))tab('downloads');call(b.dataset.action,{id:b.dataset.id||'bolt'});}});
$('source').addEventListener('change',sourceChange);
$('save-settings').addEventListener('click',()=>call('preferences',{wifiOnly:$('wifi-only').checked,origin:$('source').value==='custom'?$('custom-source').value:$('source').value}));
$('connect-images').addEventListener('click',()=>{const token=$('image-token').value.trim();if(!/^[a-fA-F0-9]{48}$/.test(token)){toast('48文字のAPIトークンを入力してください');return;}call('connectImages',{token});$('image-token').value='';});
function refresh(){try{if(window.Atelier)render(JSON.parse(Atelier.getState()));}catch(_){toast('表示を更新できませんでした。画面を開き直してください。');}}
refresh();setInterval(refresh,1000);
