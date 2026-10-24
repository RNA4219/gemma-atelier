"""Termux内で実行する、再開可能なBolt/NPUセットアップ。CPUへの代替実行はしない。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
import urllib.request
from urllib.error import HTTPError

MODEL_URL = ('https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-E4B-4GB/resolve/'
             '4008514238a0b4ae17af3988818f75bf084541a5/Nemotron-3.5-Bolt-E4B-Q4_K_M.gguf')
MODEL_SHA = '5442bfb3b1384ff3774c9989e9457dad81bbb5830fbcf96adfdde40078831d5a'
MODEL_BYTES = 6180446208
GIB = 1024**3


def digest(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for b in iter(lambda: f.read(4 << 20), b''): h.update(b)
    return h.hexdigest()


def fetch(url, path, expected_size, expected_sha, progress=lambda n: None):
    """Range再開。応答が200なら先頭から、206は開始位置を照合する。"""
    path = Path(path)
    if path.exists():
        if path.stat().st_size == expected_size and digest(path) == expected_sha: return
        raise ValueError('保存済みファイルの内容が一致しません: ' + path.name)
    part = path.with_suffix(path.suffix + '.part')
    for attempt in range(4):
        start = part.stat().st_size if part.exists() else 0
        if start > expected_size: raise ValueError('ダウンロード途中のファイルサイズが不正です')
        if start == expected_size: break
        try:
            req = urllib.request.Request(url, headers={'Range': f'bytes={start}-', 'User-Agent': 'Gemma-Atelier-Setup/1.1'})
            with urllib.request.urlopen(req, timeout=45) as response:
                if response.status == 206:
                    if not response.headers.get('Content-Range', '').startswith(f'bytes {start}-'):
                        raise ValueError('ダウンロード再開位置が一致しません')
                    mode = 'ab'
                elif response.status == 200: start = 0; mode = 'wb'
                else: raise ValueError('ダウンロード元の応答が不正です')
                last = 0
                with part.open(mode) as out:
                    while True:
                        block = response.read(1 << 20)
                        if not block: break
                        out.write(block); start += len(block)
                        if start > expected_size: raise ValueError('ダウンロード内容が予定サイズを超えました')
                        if time.monotonic() - last > 2: progress(start); last = time.monotonic()
            if start == expected_size: break
            raise OSError('通信が途中で終了しました')
        except (OSError, HTTPError):
            if attempt == 3: raise
            time.sleep(2 + attempt * 2)
    if not part.exists() or part.stat().st_size != expected_size or digest(part) != expected_sha:
        raise ValueError('SHA256またはサイズが一致しません。途中ファイルを確認して再試行してください')
    os.replace(part, path)


def check_device(getprop, ram):
    soc = getprop('ro.soc.model').strip().upper()
    if not soc.startswith('SM8650'):
        raise ValueError('この版はSnapdragon 8 Gen 3 (SM8650 / Hexagon v75)専用です: ' + soc)
    if ram < 14 * GIB: raise ValueError('この版はRAM 16GB搭載端末向けです')
    if int(getprop('ro.build.version.sdk')) < 31: raise ValueError('Android 12以上が必要です')


def verify_npu(log):
    for required in ['libggml-htp-v75.so', 'offloaded 43/43 layers', 'PLE integrity enabled']:
        if required not in log: raise RuntimeError('NPU起動を確認できません: ' + required)


def atomic_copy(source, target, sha, executable=False):
    target.parent.mkdir(parents=True, exist_ok=True)
    if digest(source) != sha: raise ValueError('同梱ファイルの整合性エラー: ' + source.name)
    if target.exists() and digest(target) == sha: return
    temporary = target.with_name(target.name + '.setup-new')
    backup = target.with_name(target.name + '.before-setup')
    if target.exists() and not backup.exists(): shutil.copyfile(target, backup)
    shutil.copyfile(source, temporary)
    temporary.chmod(0o700 if executable else 0o600)
    os.replace(temporary, target)
    return True


def installed_model(folder):
    try:
        m = json.loads((folder / 'manifest.json').read_text())
        return (m['source_sha256'] == MODEL_SHA and
                {e['file'] for e in m['files']} == {'core.gguf', 'ple.gguf'} and
                all((folder / e['file']).stat().st_size == e['bytes'] for e in m['files']))
    except (OSError, ValueError, KeyError, TypeError): return False


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--callback', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'http://127\.0\.0\.1:\d+/[0-9a-f]{48}', args.callback):
        raise ValueError('ローカルのセットアップ接続先が不正です')
    bundle = Path(__file__).resolve().parent
    root = Path.home() / 'gemma-e4b'
    root.mkdir(exist_ok=True)
    import fcntl
    lock = (root / '.setup.lock').open('w')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    def report(message, state='running'):
        print(message, flush=True)
        data = json.dumps({'message': message, 'state': state}, ensure_ascii=False).encode()
        try: urllib.request.urlopen(urllib.request.Request(args.callback + '/progress', data=data, headers={'Content-Type':'application/json'}), timeout=4).close()
        except OSError: pass  # Androidが起動画面を止めても、モデルの取得は続けられる。
    try:
        ram = int(re.search(r'MemTotal:\s+(\d+)', Path('/proc/meminfo').read_text()).group(1)) * 1024
        check_device(lambda k: subprocess.check_output(['/system/bin/getprop', k], text=True), ram)
        report('対応端末を確認しました。NPUエンジンを準備しています。')
        contents = json.loads((bundle / 'manifest.json').read_text())
        bridge_changed = False
        for entry in contents['files']:
            name = entry['path']
            if Path(name).is_absolute() or '..' in Path(name).parts: raise ValueError('同梱パスが不正です')
            changed = atomic_copy(bundle / 'payload' / name, root / name, entry['sha256'], entry.get('executable', False))
            if name == 'bin/chat-bridge-bolt': bridge_changed = bool(changed)
        folder = root / 'models/nemotron-bolt-e4b'
        if not installed_model(folder):
            if folder.exists() and any(folder.iterdir()):
                raise ValueError('既存モデルを確認できません。上書きせず停止しました。')
            if shutil.disk_usage(root).free < 16 * GIB:
                raise ValueError('Boltの取得・分離には空き容量16GiB以上が必要です')
            downloads = root / '.setup-downloads'; downloads.mkdir(exist_ok=True)
            original = downloads / 'model.gguf'
            report('Boltを公式配布元から取得します（約6.2GB）。中断しても再開できます。')
            fetch(MODEL_URL, original, MODEL_BYTES, MODEL_SHA,
                  lambda n: report(f'Bolt取得中: {n / MODEL_BYTES:.0%} ({n / GIB:.2f} GiB)'))
            report('取得したモデルを確認しました。PLEを別ファイルに分離しています。')
            sys.path.insert(0, str(bundle))
            from split_bolt_ple import split
            split_folder = downloads / 'split'
            split_folder.mkdir(exist_ok=True)
            # このインストーラーが作った途中出力だけを再作成する。
            for name in ['core.gguf','ple.gguf','manifest.json','bolt-template.jinja']:
                (split_folder / name).unlink(missing_ok=True)
            split(original, split_folder)
            folder.parent.mkdir(parents=True, exist_ok=True)
            if folder.exists(): folder.rmdir()  # 上で空ディレクトリと確認したものだけ。
            os.replace(split_folder, folder)
            original.unlink()  # インストーラー自身が取得した原本。分離と照合の成功後だけ解放。
        else: report('既存のBolt・PLEを利用します。モデルを再ダウンロードしません。')
        settings = Path.home() / '.termux/termux.properties'
        settings.parent.mkdir(exist_ok=True)
        text = settings.read_text() if settings.exists() else ''
        before = text
        if re.search(r'(?m)^\s*allow-external-apps\s*=', text):
            text = re.sub(r'(?m)^\s*allow-external-apps\s*=.*$', 'allow-external-apps = true', text)
        else: text += '\nallow-external-apps = true\n'
        if text != before:
            backup = root / '.setup-termux-properties.backup'
            if not backup.exists(): backup.write_text(before)
            settings.write_text(text)
        subprocess.run(['termux-reload-settings'], check=False)
        boot = Path.home() / '.termux/boot'; boot.mkdir(parents=True, exist_ok=True)
        boot_script = boot / '30-gemma-atelier'
        boot_script.write_text('#!/data/data/com.termux/files/usr/bin/sh\nexec sh "$HOME/gemma-e4b/boot-atelier.sh"\n')
        boot_script.chmod(0o700)
        env = root / 'atelier-llm.env'
        if not env.exists(): env.write_text('ATELIER_LLM_PROFILE=bolt\nATELIER_LLM_THINKING=off\nATELIER_LLM_FALLBACK=none\nATELIER_IMAGES_OPTIONAL=1\n')
        report('BoltをNPUで起動して確認しています。CPUには切り替えません。')
        subprocess.run(['sh', str(root / 'start-bolt.sh')], check=True, timeout=240)
        verify_npu((root / 'bolt-server.log').read_text(errors='replace'))
        entrypoint = 'restart-after-setup.sh' if bridge_changed else 'start-atelier.sh'
        subprocess.run(['sh', str(root / entrypoint)], check=True, timeout=90)
        status = json.load(urllib.request.urlopen('http://127.0.0.1:8085/api/status', timeout=8))
        if not status.get('gemma'): raise RuntimeError('会話APIの接続を確認できません')
        (root / 'setup-complete.json').write_text(json.dumps({'version': '1.1.0', 'npu': 'HTP0-v75', 'model': 'Bolt-E4B', 'images': status.get('images',False)}))
        report('BoltのNPU準備が完了しました。Gemmaへ戻り「会話を開く」を押してください。', 'complete')
    except Exception as error:
        report('セットアップを停止しました: ' + str(error), 'error')
        raise


if __name__ == '__main__': main()
