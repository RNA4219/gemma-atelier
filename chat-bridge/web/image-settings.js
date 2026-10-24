'use strict';
const imageSettingsUI = (() => {
  const el = id => document.getElementById(id);
  const key = 'gemma-atelier-image-settings-v1';
  const quality = 'low quality, blurry, text, watermark, bad anatomy';
  const builtins = [
    {id:'fast', name:'高速・ネガティブなし', negative:'', cfg:1},
    {id:'quality', name:'品質補助', negative:quality, cfg:1.5},
    {id:'matte', name:'テカリを抑える', negative:quality+', shiny skin, shiny hair, shiny clothes, glossy, latex, patent leather, plastic, metallic, specular highlights, 3d, realistic, photorealistic', cfg:1.5}
  ];
  let custom = [], selected = 'fast', locked = false;
  const valid = s => s && typeof s.negative === 'string' && s.negative.length <= 2000 && new TextEncoder().encode(s.negative).length <= 3000 && Number.isFinite(s.cfg) && s.cfg >= 1 && s.cfg <= 3;
  let initial = builtins[0];
  try {
    const saved = JSON.parse(localStorage.getItem(key) || 'null');
    if (saved && Array.isArray(saved.presets)) {
      const ids = new Set();
      custom = saved.presets.filter(p => valid(p) && typeof p.id === 'string' && p.id.startsWith('user-') && typeof p.name === 'string' && p.name.trim() && p.name.length <= 40 && !ids.has(p.id) && ids.add(p.id)).slice(0,20);
    }
    if (valid(saved)) initial = saved;
    if ([...builtins,...custom].some(p => p.id === saved?.selected)) selected = saved.selected;
  } catch {}
  function current() {
    const value = {negative:el('negative').value, cfg:Number(el('cfg').value)};
    if (!valid(value) || !el('cfg').value.trim()) throw Error('ネガティブは3000バイト以内、CFGは1〜3で入力してください。');
    return value;
  }
  function update() {
    const cfg = Number(el('cfg').value);
    el('cfg-note').textContent = cfg === 1 ? 'CFG 1ではネガティブは効きません。使う場合は1.5から試せます。' : 'DMD2はCFG 1が配布時の設定です。上げるとネガティブが効き、絵柄や生成時間も変わります。';
    const preset = [...builtins,...custom].find(p => p.id === selected);
    const matches = preset && preset.negative === el('negative').value && preset.cfg === cfg;
    el('settings-summary').textContent = (matches ? preset.name : '手動設定')+' · CFG '+el('cfg').value;
    el('delete-preset').disabled = locked || !custom.some(p => p.id === selected);
  }
  function persist() {
    update();
    try {
      const value = current();
      localStorage.setItem(key, JSON.stringify({...value, selected, presets:custom}));
    } catch (err) {
      el('preset-status').textContent = err.message || '設定を保存できませんでした。';
    }
  }
  function renderOptions() {
    el('negative-preset').replaceChildren();
    for (const p of [...builtins,...custom]) {
      const option = document.createElement('option');
      option.value = p.id; option.textContent = p.name+(p.id.startsWith('user-')?'（保存済み）':'');
      el('negative-preset').append(option);
    }
    el('negative-preset').value = selected;
  }
  function apply(p) {
    selected = p.id;
    el('negative').value = p.negative; el('cfg').value = p.cfg;
    el('preset-name').value = p.id.startsWith('user-') ? p.name : '';
    el('negative-preset').value = p.id;
    persist();
  }
  el('negative').value = initial.negative; el('cfg').value = initial.cfg;
  renderOptions();
  el('preset-name').value = custom.find(p => p.id === selected)?.name || '';
  el('negative-preset').addEventListener('change', () => {
    apply([...builtins,...custom].find(p => p.id === el('negative-preset').value));
    el('preset-status').textContent = 'プリセットを適用しました。自由に編集できます。';
  });
  for (const id of ['negative','cfg']) el(id).addEventListener('input', persist);
  el('save-preset').addEventListener('click', () => {
    try {
      const value = current(), name = el('preset-name').value.trim();
      if (!name || name.length > 40) throw Error('プリセット名を1〜40文字で入力してください。');
      if (builtins.some(p => p.name === name)) throw Error('用意されたプリセットと別の名前を付けてください。');
      const existing = custom.find(p => p.name === name);
      if (!existing && custom.length >= 20) throw Error('保存できるプリセットは20件までです。');
      const p = {id:existing?.id || 'user-'+crypto.randomUUID(), name, ...value};
      // 保存に失敗した場合は一覧の状態も変えない。
      const next = existing ? custom.map(item => item.id === existing.id ? p : item) : [...custom,p];
      localStorage.setItem(key, JSON.stringify({...value, selected:p.id, presets:next}));
      custom = next; selected = p.id; renderOptions(); update();
      el('preset-status').textContent = existing ? '同じ名前のプリセットを更新しました。' : 'プリセットを保存しました。';
    } catch (err) { el('preset-status').textContent = err.message; }
  });
  el('delete-preset').addEventListener('click', () => {
    if (!custom.some(p => p.id === selected)) return;
    try {
      const next = custom.filter(p => p.id !== selected);
      const value = current();
      localStorage.setItem(key, JSON.stringify({...value, selected:'fast', presets:next}));
      custom = next; selected = 'fast'; el('preset-name').value = ''; renderOptions(); update();
      el('preset-status').textContent = 'プリセットを削除しました。現在の入力は残しています。';
    } catch (err) { el('preset-status').textContent = err.message; }
  });
  update();
  return {current, setBusy(value) {
    locked = value;
    for (const id of ['negative-preset','negative','cfg','preset-name','save-preset']) el(id).disabled = value;
    update();
  }};
})();
