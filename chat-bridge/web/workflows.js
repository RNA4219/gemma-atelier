'use strict';
(() => {
 const el=id=>document.getElementById(id), key='gemma-atelier-workflows-v1';
 const example='a small red fox sitting in a snowy forest, watercolor illustration, matte texture, soft daylight';
 const ill='Illustrious-v16-DMD2-SDXL-8Gen3', photo='CyberRealistic-v10-DMD2-SDXL-8Gen3';
 let graph, selected='', library=[], lastResult=null, catalog=[], adapters=[], ready=false, controller=null, zoom=1;
 const swap='community_rinflanime_v50_swap';
 let controlNetReady=false,controlNetReason='ControlNetの実行対応を確認しています。';
 const statuses=new Map();
 const copy=value=>JSON.parse(JSON.stringify(value));
 const title=n=>({'core.prompt':'プロンプト','sdxl.sample':'画像生成','core.output':'画像出力','core.image':'参照画像'}[n.type]||n.type)+' · '+n.id;
 function node(type,id,x,y,params={},inputs={}){return {id,type,x,y,params,inputs};}
 function sample(id,model,x,y,prompt,image){return node('sdxl.sample',id,x,y,{model,steps:model===swap?'20':'8',cfg:model===swap?'5':'1.5',seed:'0',denoise:'0.35',scheduler:model===swap?'dpm':'lcm',width:'1024',height:'1024',aspect:'1:1'},{prompt,...(image?{image}:{})});}
 function template(mode){
  let negative='',cfg='1.5';try{const current=imageSettingsUI.current();negative=current.negative;cfg=String(current.cfg);}catch{}
  const p=node('core.prompt','words',24,30,{prompt:example,negative});
  const a=sample('draw',ill,260,30,p.id);a.params.cfg=cfg;
  const out=node('core.output','first',500,30,{}, {media:a.id});
  const nodes=[p,a,out];
  if(mode==='control'){Object.assign(a.params,{model:swap,steps:'20',cfg:'5',scheduler:'dpm',controlnet:'openpose',control_strength:'0.8'});a.inputs.control='reference';nodes.push(node('core.image','reference',24,230,{image:''}));}
  if(mode==='branch'){const b=sample('photo',photo,260,230,p.id);b.params.cfg=cfg;nodes.push(b,node('core.output','second',500,230,{}, {media:b.id}));}
  if(mode==='chain'){const q=node('core.prompt','finish_words',24,260,{prompt:example,negative});const b=sample('finish',photo,500,260,q.id,a.id);b.params.cfg=cfg;nodes.push(q,b,node('core.output','second',740,260,{}, {media:b.id}));}
  return {format:1,nodes};
 }
 function parseGraph(input){
  const g=copy(input);
  if(g.format!==1||!Array.isArray(g.nodes)||g.nodes.length<1||g.nodes.length>12)throw Error('LLM Atelier形式の1〜12ノードを読み込んでください。ComfyUIのJSONは読み込めません。');
  if(Object.keys(g).some(k=>!['format','nodes'].includes(k)))throw Error('このJSONには未対応の設定があります。元のファイルは変更していません。');
  const ids=new Set();
  for(const n of g.nodes){
   if(!n||!/^([A-Za-z][A-Za-z0-9_-]{0,47})$/.test(n.id)||ids.has(n.id)||!['core.prompt','sdxl.sample','core.output','core.image'].includes(n.type))throw Error('未対応のノードか、重複したIDがあります。');
   ids.add(n.id);
   if(Object.keys(n).some(k=>!['id','type','params','inputs','x','y'].includes(k)))throw Error('ノードに未対応の項目があります。');
   for(const part of ['params','inputs'])if(!n[part]||Array.isArray(n[part])||typeof n[part]!=='object'||Object.values(n[part]).some(v=>typeof v!=='string'))throw Error('ノードの設定・接続を確認してください。');
   if(!Number.isFinite(n.x)||!Number.isFinite(n.y)||n.x<0||n.y<0||n.x>10000||n.y>10000)throw Error('ノード位置が不正です。');
  }
  return g;
 }
 function error(message){el('wf-error').textContent=message;}
 function persist(){try{localStorage.setItem(key,JSON.stringify({graph,selected,name:el('wf-name').value,library,lastResult,zoom}));}catch{error('ブラウザへ保存できません。JSONを書き出して保管してください。');}}
 try{
  const saved=JSON.parse(localStorage.getItem(key)||'null');
  if(saved){graph=parseGraph(saved.graph);selected=saved.selected;zoom=Number.isFinite(saved.zoom)?Math.min(1.5,Math.max(.25,saved.zoom)):1;el('wf-name').value=String(saved.name||'').slice(0,40);library=Array.isArray(saved.library)?saved.library.slice(0,20).filter(item=>item&&typeof item.name==='string'&&item.graph):[];lastResult=saved.lastResult;}
 }catch{error('保存データを読み取れません。既存データは自動で上書きしません。');}
 if(!graph)graph=template('single');
 if(!graph.nodes.some(n=>n.id===selected))selected=graph.nodes[0].id;
 function activate(workflow){el('workflow-view').hidden=!workflow;el('chat-view').hidden=workflow;el('reset').hidden=workflow;el('view-chat').classList.toggle('active',!workflow);el('view-workflow').classList.toggle('active',workflow);el('view-chat').setAttribute('aria-pressed',String(!workflow));el('view-workflow').setAttribute('aria-pressed',String(workflow));if(workflow){renderGraph();if(!ready)loadCatalog();}}
 el('view-chat').onclick=()=>activate(false);el('view-workflow').onclick=()=>activate(true);
 function element(tag,text,className){const e=document.createElement(tag);if(text!==undefined)e.textContent=text;if(className)e.className=className;return e;}
 function button(text,fn,className='quiet'){const e=element('button',text,className);e.type='button';e.onclick=fn;return e;}
 function compatible(){return catalog.filter(m=>m.supported);}
 function syncControls(){
  el('workflow-view').querySelectorAll('button,input,select,textarea').forEach(e=>e.disabled=busy||e.dataset.inactive==='true');
  const blockedControl=!controlNetReady&&graph.nodes.some(n=>n.type==='sdxl.sample'&&n.params.controlnet&&n.params.controlnet!=='none');
  el('wf-run').disabled=busy||!ready||blockedControl;el('wf-stop').disabled=!controller;
  el('wf-control-status').textContent=blockedControl?controlNetReason:'';
  el('wf-load').disabled=busy||!library.length;el('wf-delete').disabled=busy||!library.length;
 }
 window.addEventListener('atelier-busy',syncControls);
 async function loadCatalog(){
  try{const r=await fetch('/api/workflows/catalog',{cache:'no-store'});if(!r.ok)throw Error(await r.text());const data=await r.json();catalog=data.models;adapters=data.adapters||[];controlNetReady=data.controlnet?.enabled===true;controlNetReason=data.controlnet?.reason||'ControlNetは準備中です。';ready=compatible().length>0;el('wf-capabilities').textContent=data.note;el('wf-state').textContent=ready?`${compatible().length}モデルに接続済み · 順番に実行`:'対応モデルが見つかりません';renderInspector();renderAssets();}
  catch(err){ready=false;el('wf-state').textContent='画像エンジンに接続できません';error(err.message);}
  syncControls();
 }
 function drawLinks(){
  const svg=el('wf-links');svg.replaceChildren();
  for(const target of graph.nodes)for(const [port,id] of Object.entries(target.inputs)){
   const source=graph.nodes.find(n=>n.id===id);if(!source)continue;
   const x1=source.x+190,y1=source.y+52,x2=target.x,y2=target.y+52;
   const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',`M ${x1} ${y1} C ${x1+65} ${y1}, ${x2-65} ${y2}, ${x2} ${y2}`);path.setAttribute('class','wf-wire'+(port==='prompt'?'':' image'));svg.append(path);
  }
 }
 function renderGraph(){
  const width=Math.max(780,...graph.nodes.map(n=>n.x+240)),height=Math.max(460,...graph.nodes.map(n=>n.y+160));
  el('wf-canvas').style.width=width+'px';el('wf-canvas').style.height=height+'px';
  el('wf-canvas').style.zoom=zoom;el('wf-zoom-label').textContent=Math.round(zoom*100)+'%';
  const container=el('wf-nodes');container.replaceChildren();
  for(const n of graph.nodes){
   const card=button('',()=>{selected=n.id;renderGraph();renderInspector();persist();},'wf-node '+(n.type==='sdxl.sample'?'sample':n.type==='core.output'?'output':'prompt'));
   card.dataset.node=n.id;card.setAttribute('aria-label',title(n));card.classList.toggle('selected',selected===n.id);card.classList.toggle('executing',statuses.get(n.id)==='running');card.classList.toggle('complete',statuses.get(n.id)==='done');card.style.left=n.x+'px';card.style.top=n.y+'px';
   const head=element('span',title(n),'wf-node-head');
   let detail=n.type==='core.prompt'?n.params.prompt:n.type==='sdxl.sample'?((catalog.find(m=>m.id===n.params.model)?.label||n.params.model)+'\n'+(n.inputs.image?'画像から仕上げ':'文章から生成')):'← '+(n.inputs.media||'未接続');
   if(n.type==='core.image')detail=n.params.image?'選択済み · 画像を変更できます':'タップして画像を選択';
   if(n.type==='sdxl.sample'&&n.params.controlnet&&n.params.controlnet!=='none')detail+='\nControlNet · '+n.params.controlnet;
   card.append(head,element('span',detail,'wf-node-body'));container.append(card);
   let drag=null,moved=false;
   head.onpointerdown=e=>{if(busy)return;drag={x:e.clientX,y:e.clientY,nx:n.x,ny:n.y};moved=false;head.setPointerCapture(e.pointerId);};
   head.onpointermove=e=>{if(!drag)return;const dx=(e.clientX-drag.x)/zoom,dy=(e.clientY-drag.y)/zoom;if(Math.abs(dx)+Math.abs(dy)>4)moved=true;n.x=Math.max(8,Math.min(9000,drag.nx+dx));n.y=Math.max(8,Math.min(9000,drag.ny+dy));card.style.left=n.x+'px';card.style.top=n.y+'px';drawLinks();};
   head.onpointerup=()=>{if(!drag)return;drag=null;if(moved){persist();requestAnimationFrame(renderGraph);}};
   head.onpointercancel=()=>{drag=null;persist();};
  }
  drawLinks();syncControls();
 }
 function field(label,control,parent){const wrapper=element('label',label);wrapper.append(control);parent.append(wrapper);return control;}
 function selectInput(options,value,onchange){const select=element('select');for(const [id,label,disabled] of options){const o=element('option',label);o.value=id;o.disabled=!!disabled;select.append(o);}select.value=value;select.onchange=()=>onchange(select.value);return select;}
 function change(n,key,value){n.params[key]=value;persist();renderGraph();}
 function renderInspector(){
  const panel=el('wf-inspector');panel.replaceChildren();const n=graph.nodes.find(n=>n.id===selected);if(!n){panel.textContent='ノードを選んでください。';return;}
  const heading=element('div',undefined,'wf-row');heading.append(element('h3',title(n)),button('ノードを削除',()=>{if(graph.nodes.length===1){error('最後のノードは削除できません。テンプレートから作り直せます。');return;}graph.nodes=graph.nodes.filter(other=>other.id!==n.id);for(const other of graph.nodes)for(const [port,id] of Object.entries(other.inputs))if(id===n.id)delete other.inputs[port];selected=graph.nodes[0]?.id||'';persist();renderGraph();renderInspector();}));panel.append(heading);
  if(n.type==='core.prompt'){
   for(const [key,label] of [['prompt','プロンプト（英語・生成用タグ）'],['negative','ネガティブプロンプト']]){const input=element('textarea');input.value=n.params[key]||'';input.rows=3;input.maxLength=2000;input.oninput=()=>change(n,key,input.value);field(label,input,panel);}
   panel.append(button('会話側のネガティブを取り込む',()=>{try{change(n,'negative',imageSettingsUI.current().negative);renderInspector();}catch(err){error(err.message);}}));
  }else if(n.type==='sdxl.sample'){
   const options=catalog.map(m=>[m.id,m.label+(m.supported?'':' · NPU未確認'),!m.supported]);if(!options.some(o=>o[0]===n.params.model))options.push([n.params.model,n.params.model+' · 未接続',true]);
   field('この段階のモデル',selectInput(options,n.params.model,value=>{const m=catalog.find(m=>m.id===value);n.params.model=value;n.params.steps=String(m?.steps||20);n.params.cfg=String(m?.cfg||5);n.params.scheduler=value===swap?(m?.scheduler||'euler_a'):'lcm';if(value!==swap){delete n.params.loras;clearControl(n);el('wf-state').textContent='通常モデルへ変更したためLoRAとControlNetを解除しました。';}persist();renderGraph();renderInspector();}),panel);
   connection('prompt','プロンプトの接続','core.prompt',false);connection('image','画像の接続（つなぐとimg2img）','sdxl.sample',true);
   const grid=element('div',undefined,'wf-fields');panel.append(grid);
   const isSwap=n.params.model===swap;
   for(const [key,label,min,max,step] of [['steps','ステップ',1,isSwap?40:30,1],['cfg','CFG',1,isSwap?10:3,.1],['seed','Seed（0でランダム）',0,2147483647,1],['denoise','再描画の強さ',0,1,.05]]){const input=element('input');input.type='number';input.value=n.params[key];input.min=min;input.max=max;input.step=step;input.oninput=()=>change(n,key,input.value);field(label,input,grid);}
   field('サンプラー',selectInput((isSwap?['euler_a','euler','dpm','lcm']:['lcm']).map(s=>[s,s]),n.params.scheduler,value=>change(n,'scheduler',value)),panel);
   panel.append(element('p','1024 × 1024 · VAE：モデル内蔵のNPU版。再描画の強さは画像接続時に使います。CFG 1ではネガティブが効きません。','wf-note'));
   if(isSwap){renderControl(n,panel);renderLoras(n,panel);}else panel.append(element('p','LoRA・ControlNetを使うには「LoRA対応 · rinFlanime Illustrious」を選んでください。','wf-note'));
  }else if(n.type==='core.image'){
   panel.append(element('p','写真・イラスト・骨格図を選び、生成ノードのControlNetへ接続します。画像はこの端末に保存します。','wf-note'));
   renderImagePicker(n,panel);
  }else{connection('media','表示する画像','sdxl.sample',false);panel.append(element('p','出力ノードごとに完成画像を保存します。途中の生成にも出力をつなぐと、仕上げ前後を確認できます。','wf-note'));}
  function connection(port,label,type,optional){const choices=[['',optional?'接続なし（文章から生成）':'接続先を選択'],...graph.nodes.filter(other=>(other.type===type||(port==='image'&&other.type==='core.image'))&&other.id!==n.id).map(other=>[other.id,title(other)])];field(label,selectInput(choices,n.inputs[port]||'',value=>{if(value)n.inputs[port]=value;else delete n.inputs[port];persist();renderGraph();}),panel);}
  syncControls();
 }
 function freshID(base){let i=1;while(graph.nodes.some(n=>n.id===base+i))i++;return base+i;}
 function clearControl(n){
  const old=n.inputs.control;delete n.inputs.control;delete n.params.control_strength;delete n.params.controlnet;
  if(old&&!graph.nodes.some(other=>Object.values(other.inputs).includes(old)))graph.nodes=graph.nodes.filter(other=>other.id!==old||other.type!=='core.image');
 }
 function renderImagePicker(n,panel){
  const input=element('input');input.type='file';input.accept='image/png,image/jpeg,image/webp';input.className='wf-image-upload';input.setAttribute('aria-label','参照画像を選択');
  input.onchange=async()=>{const file=input.files[0];if(!file)return;try{
   if(file.size>20*1024*1024)throw Error('元画像は20MB以内にしてください。');setBusy(true);error('');el('wf-state').textContent='参照画像を端末に保存しています…';
   const bitmap=await createImageBitmap(file);if(!bitmap.width||!bitmap.height){bitmap.close();throw Error('画像を読み取れません。');}
   const scale=Math.min(1,1024/Math.max(bitmap.width,bitmap.height)),canvas=document.createElement('canvas');canvas.width=Math.max(1,Math.round(bitmap.width*scale));canvas.height=Math.max(1,Math.round(bitmap.height*scale));canvas.getContext('2d').drawImage(bitmap,0,0,canvas.width,canvas.height);bitmap.close();
   const data=canvas.toDataURL('image/png').split(',')[1];const response=await fetch('/api/workflows/image',{method:'POST',headers:{'Content-Type':'application/json','X-Chat-Client':'1'},body:JSON.stringify({png:data})});if(!response.ok)throw Error(await response.text());
   n.params.image=(await response.json()).image;persist();renderGraph();renderInspector();el('wf-state').textContent='参照画像を保存しました。ControlNetの種類と強度を設定して実行できます。';
  }catch(err){error(err.message);}finally{setBusy(false);}};
  panel.append(input);
  if(/^\/images\/[a-f0-9]{24}\.png$/.test(n.params.image||'')){const img=element('img');img.src=n.params.image;img.alt='ControlNetの参照画像';img.className='wf-reference-preview';panel.append(img);}
 }
 function renderControl(n,panel){
  const section=element('div',undefined,'wf-control');section.append(element('h4','ControlNet · '+(controlNetReady?'構図・ポーズを指定':'準備中')));
  if(!controlNetReady)section.append(element('p',controlNetReason,'wf-note'));
  field('ControlNetの種類',selectInput([['none','なし'],['canny','Canny · 輪郭'],['depth','Depth · 奥行き'],['openpose','OpenPose · ポーズ']],n.params.controlnet||'none',value=>{if(value==='none')clearControl(n);else{n.params.controlnet=value;n.params.control_strength||='0.8';}persist();renderGraph();renderInspector();}),section);
  if(n.params.controlnet&&n.params.controlnet!=='none'){
   const strength=element('input');strength.type='number';strength.min=.05;strength.max=2;strength.step=.05;strength.value=n.params.control_strength;strength.oninput=()=>change(n,'control_strength',strength.value);field('ControlNetの強度',strength,section);
   const choices=[['','参照画像を接続'],...graph.nodes.filter(other=>['core.image','sdxl.sample'].includes(other.type)&&other.id!==n.id).map(other=>[other.id,title(other)])];
   field('ControlNetの参照接続',selectInput(choices,n.inputs.control||'',value=>{if(value)n.inputs.control=value;else delete n.inputs.control;persist();renderGraph();renderInspector();}),section);
   const ref=graph.nodes.find(other=>other.id===n.inputs.control);
   if(ref?.type==='core.image')renderImagePicker(ref,section);
   else section.append(button('参照画像ノードを追加',()=>{if(graph.nodes.length>=12){error('ノードは12個までです。');return;}const image=node('core.image',freshID('reference'),24,Math.max(...graph.nodes.map(n=>n.y))+180,{image:''});graph.nodes.push(image);n.inputs.control=image.id;persist();renderGraph();renderInspector();}));
   section.append(element('p','参照画像は全体を1024角へ合わせます。写真からの骨格・奥行き抽出は端末CPU、ControlNetと画像生成はNPUを使用する設計です。JSONは画像本体を含まないため、別端末では選び直してください。','wf-note'));
   if(n.params.controlnet==='openpose')section.append(element('p','OpenPose：全身が見える写真・骨格図を推奨。採用モデルは非商用用途向けです。','wf-note'));
  }panel.append(section);
 }
 function loraEntries(n){return (n.params.loras||'').split(',').filter(Boolean).map(s=>{const at=s.lastIndexOf('@');return {name:s.slice(0,at).trim(),strength:Number(s.slice(at+1))};});}
 function renderLoras(n,panel){
  const details=element('details',undefined,'wf-loras');details.append(element('summary',`LoRAを選択 · ${loraEntries(n).length}個使用中`));
  details.append(element('p','最大4個。強度0〜2。初回は準備に時間がかかります。通常は0.7から調整してください。','wf-note'));
  for(const a of adapters.filter(a=>a.kind==='lora'&&a.supported)){
   const row=element('div',undefined,'wf-lora-row'),check=element('input'),strength=element('input');check.type='checkbox';check.checked=loraEntries(n).some(e=>e.name===a.name);check.dataset.lora=a.name;
   const label=element('label');label.append(check,element('span',a.name.replace(/\.safetensors$/,'')));row.append(label);
   strength.type='number';strength.min=0;strength.max=2;strength.step=.05;strength.value=loraEntries(n).find(e=>e.name===a.name)?.strength??.7;strength.setAttribute('aria-label',a.name+' の強度');strength.dataset.inactive=String(!check.checked);field('強度',strength,row);
   const update=()=>{const entries=loraEntries(n).filter(e=>e.name!==a.name);if(check.checked){if(entries.length>=4){check.checked=false;error('LoRAは4個までです。');return;}entries.push({name:a.name,strength:strength.value});}n.params.loras=entries.map(e=>e.name+'@'+e.strength).join(',');strength.dataset.inactive=String(!check.checked);details.querySelector('summary').textContent=`LoRAを選択 · ${entries.length}個使用中`;persist();renderGraph();};
   check.onchange=update;strength.oninput=update;row.append(element('p',a.note,'wf-note'));details.append(row);
  }
  panel.append(details);
 }
 function renderAssets(){const target=el('wf-assets-list');target.replaceChildren();for(const a of adapters){const row=element('div',undefined,'wf-asset');row.append(element('strong',a.name),element('p',`${a.kind==='vae'?'VAE':'LoRA'} · ${a.family} · ${a.supported?'導入済み':'未対応'} · ${(a.bytes/1048576).toFixed(1)} MB`),element('p',a.note,'wf-note'));target.append(row);}}
 function add(type){if(busy)return;if(graph.nodes.length>=12){error('ノードは12個までです。');return;}const id=freshID(type==='core.prompt'?'words':type==='sdxl.sample'?'draw':'output'),y=30+Math.floor(graph.nodes.length/3)*180;let n;
  if(type==='core.prompt')n=node(type,id,24,y,{prompt:example,negative:''});
  else if(type==='core.image')n=node(type,id,24,y,{image:''});
  else if(type==='sdxl.sample')n=sample(id,compatible()[0]?.id||ill,260,y,graph.nodes.find(n=>n.type==='core.prompt')?.id||'');
  else n=node(type,id,500,y,{}, {media:graph.nodes.filter(n=>n.type==='sdxl.sample').at(-1)?.id||''});
  graph.nodes.push(n);selected=id;persist();renderGraph();renderInspector();el('wf-viewport').scrollTo({top:y-25,left:Math.max(0,n.x-25),behavior:'smooth'});
 }
 el('wf-add-prompt').onclick=()=>add('core.prompt');el('wf-add-sample').onclick=()=>add('sdxl.sample');el('wf-add-output').onclick=()=>add('core.output');
 el('wf-add-image').onclick=()=>add('core.image');
 function setZoom(value){zoom=Math.min(1.5,Math.max(.25,value));persist();renderGraph();}
 el('wf-zoom-in').onclick=()=>setZoom(zoom+.15);el('wf-zoom-out').onclick=()=>setZoom(zoom-.15);
 el('wf-fit').onclick=()=>{const width=Math.max(780,...graph.nodes.map(n=>n.x+240));const height=Math.max(460,...graph.nodes.map(n=>n.y+160));setZoom(Math.min(1,(el('wf-viewport').clientWidth-16)/width,(el('wf-viewport').clientHeight-16)/height));el('wf-viewport').scrollTo(0,0);};
 el('wf-new').onclick=()=>{if(busy)return;graph=template(el('wf-template').value);selected=graph.nodes[0].id;statuses.clear();error('');el('wf-name').value='';persist();renderGraph();renderInspector();el('wf-viewport').scrollTo(0,0);};
 function renderLibrary(){const select=el('wf-saved'),previous=select.value;select.replaceChildren();for(const item of library){const o=element('option',item.name);o.value=item.name;select.append(o);}if(library.some(x=>x.name===previous))select.value=previous;syncControls();}
 el('wf-save').onclick=()=>{const name=el('wf-name').value.trim();if(!name){error('保存する名前を入力してください。');return;}const at=library.findIndex(item=>item.name===name);if(at<0&&library.length>=20){error('保存できるフローは20件までです。');return;}const entry={name,graph:copy(graph)};if(at>=0)library[at]=entry;else library.push(entry);persist();renderLibrary();el('wf-saved').value=name;el('wf-state').textContent='保存しました：'+name;error('');};
 el('wf-load').onclick=()=>{try{const entry=library.find(item=>item.name===el('wf-saved').value);if(!entry)return;graph=parseGraph(entry.graph);selected=graph.nodes[0].id;el('wf-name').value=entry.name;statuses.clear();persist();renderGraph();renderInspector();error('');}catch(err){error(err.message);}};
 el('wf-delete').onclick=()=>{const name=el('wf-saved').value;library=library.filter(item=>item.name!==name);persist();renderLibrary();el('wf-state').textContent='保存フローを削除しました。編集中の内容は残ります。';};
 function download(g,name){const blob=new Blob([JSON.stringify(g,null,2)],{type:'application/json'}),url=URL.createObjectURL(blob),a=element('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),2000);}
 el('wf-export').onclick=()=>download(graph,'llm-atelier-workflow.json');
 el('wf-import').onchange=async event=>{try{const file=event.target.files[0];if(!file)return;if(file.size>96*1024)throw Error('JSONは96KB以内にしてください。');const incoming=parseGraph(JSON.parse(await file.text()));graph=incoming;selected=graph.nodes[0].id;el('wf-name').value=file.name.replace(/\.json$/i,'').slice(0,40);statuses.clear();persist();renderGraph();renderInspector();error('');}catch(err){error(err.message);}finally{event.target.value='';}};
 el('wf-from-chat').onclick=async()=>{
  const target=graph.nodes.find(n=>n.id===selected&&n.type==='core.prompt')||graph.nodes.find(n=>n.type==='core.prompt');if(!target){error('プロンプトノードを追加してください。');return;}
  let prompt=el('tags').value.trim();const text=el('prompt').value.trim();
  if(!prompt&&!text){error('会話画面に文章を入力するか、生成用タグを作ってください。');return;}
  try{setBusy(true);error('');if(!prompt){if([...text].length>600)throw Error('タグ変換する文章は600文字以内にしてください。');el('wf-state').textContent='LLM Atelierが文章を生成用タグへ変換しています…';const response=await fetch('/api/tags',{method:'POST',headers:{'Content-Type':'application/json','X-Chat-Client':'1'},body:JSON.stringify({text})});if(!response.ok)throw Error(await response.text());prompt=(await response.json()).prompt;}
   target.params.prompt=prompt;target.params.negative=imageSettingsUI.current().negative;selected=target.id;persist();renderGraph();renderInspector();el('wf-state').textContent='プロンプトへ取り込みました。編集して実行できます。';
  }catch(err){error(err.message);}finally{setBusy(false);}
 };
 function renderResults(){
  const container=el('wf-results');container.replaceChildren();if(!lastResult||!Array.isArray(lastResult.images)||!lastResult.graph)return;
  container.append(element('h3','最後の実行結果'),element('p',`${lastResult.images.length}枚 · 全体 ${Number(lastResult.seconds).toFixed(1)}秒 · 端末内で生成`,'wf-run-summary'));
  for(const result of lastResult.images){if(!/^\/images\/[a-f0-9]{24}\.png$/.test(result.image))continue;const article=element('article');article.append(element('strong','出力 · '+result.node));const img=element('img');img.src=result.image;img.alt='ワークフローの生成結果 '+result.node;article.append(img);
   const output=lastResult.graph.nodes.find(n=>n.id===result.node),source=lastResult.graph.nodes.find(n=>n.id===output?.inputs?.media),prompt=lastResult.graph.nodes.find(n=>n.id===source?.inputs?.prompt);
   if(source?.params?.controlnet&&source.params.controlnet!=='none')article.append(element('p',`ControlNet：${source.params.controlnet} · 強度 ${source.params.control_strength} · 参照 ${source.inputs.control}`,'wf-note'));
   const details=element('details');details.append(element('summary','使用したモデル・プロンプト・設定'),element('p',[source?.params?.model,prompt?.params?.prompt,'ネガティブ：'+(prompt?.params?.negative||'指定なし'),'LoRA：'+(source?.params?.loras||'なし'),'VAE：モデル内蔵（NPU）',source?'CFG '+source.params.cfg+' · steps '+source.params.steps+' · seed '+source.params.seed+' · 再描画 '+source.params.denoise+' · '+source.params.scheduler:''].filter(Boolean).join('\n')));article.append(details);
   const save=element('a','画像を保存','save');save.href=result.image;save.download='llm-atelier-'+result.node+'.png';article.append(save);container.append(article);
  }
  const row=element('div',undefined,'wf-result-buttons');row.append(button('この実行条件を再利用',()=>{try{graph=parseGraph(lastResult.graph);selected=graph.nodes[0].id;statuses.clear();persist();renderGraph();renderInspector();el('wf-state').textContent='実行時のseedも含めて復元しました。';}catch(err){error(err.message);}}),button('実行条件JSON',()=>download(lastResult.graph,'llm-atelier-workflow-executed.json')));container.append(row);syncControls();
 }
 el('wf-run').onclick=async()=>{
  if(busy||!ready||el('wf-run').disabled)return;controller=new AbortController();setBusy(true);error('');statuses.clear();renderGraph();el('wf-state').textContent='ワークフローを開始します…';let finished=false;
  try{
   const response=await fetch('/api/workflows/run',{method:'POST',headers:{'Content-Type':'application/json','X-Chat-Client':'1'},body:JSON.stringify({graph:copy(graph)}),signal:controller.signal});if(!response.ok)throw Error(await response.text());
   const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='';
   function consume(line){if(!line.trim())return;const event=JSON.parse(line);if(event.type==='error')throw Error(event.message);if(event.type==='status')el('wf-state').textContent=event.message;if(event.type==='node'){statuses.set(event.node,event.outcome?'done':'running');renderGraph();}if(event.type==='done'){lastResult=event;persist();renderResults();finished=true;el('wf-state').textContent=`完了 · ${event.images.length}枚 / ${event.seconds.toFixed(1)}秒`;}}
   while(true){const part=await reader.read();if(part.done)break;buffer+=decoder.decode(part.value,{stream:true});let split;while((split=buffer.indexOf('\n'))>=0){consume(buffer.slice(0,split));buffer=buffer.slice(split+1);}}
   buffer+=decoder.decode();if(buffer.trim())consume(buffer);if(!finished)throw Error('生成の途中で接続が切れました。');
  }catch(err){if(err.name==='AbortError'){el('wf-state').textContent='中止を要求しました。画像エンジンの停止処理を待ってから再実行してください。';}else{error(err.message);el('wf-state').textContent='完了しませんでした。設定と接続を確認してください。';}}
  finally{controller=null;setBusy(false);renderGraph();}
 };
 el('wf-stop').onclick=()=>controller?.abort();
 renderGraph();renderInspector();renderLibrary();renderResults();
})();
