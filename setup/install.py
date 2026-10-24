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
from download import fetch, Job, Paused, IntegrityError, source_url
import threading

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
    parser.add_argument('--attach-only', action='store_true')
    args = parser.parse_args()
    if not re.fullmatch(r'http://127\.0\.0\.1:\d+/[0-9a-f]{48}', args.callback):
        raise ValueError('ローカルのセットアップ接続先が不正です')
    bundle = Path(__file__).resolve().parent
    root = Path.home() / 'gemma-e4b'
    root.mkdir(exist_ok=True)
    import fcntl
    job = Job(root, args.callback)
    lock = (root / '.setup.lock').open('a')
    try: fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError: return  # 実行中ジョブが新しいcallbackを使って画面へ再接続する。
    if args.attach_only: raise SystemExit(2)
    report = job.report
    stop = threading.Event()
    def heartbeat():
        while not stop.wait(4): job.pulse()
    threading.Thread(target=heartbeat, daemon=True).start()
    try:
        ram = int(re.search(r'MemTotal:\s+(\d+)', Path('/proc/meminfo').read_text()).group(1)) * 1024
        check_device(lambda k: subprocess.check_output(['/system/bin/getprop', k], text=True), ram)
        settings = Path.home() / '.termux/termux.properties'
        settings.parent.mkdir(exist_ok=True)
        previous = settings.read_text() if settings.exists() else ''
        text = re.sub(r'(?m)^\s*allow-external-apps\s*=.*$', 'allow-external-apps = true', previous)
        if not re.search(r'(?m)^\s*allow-external-apps\s*=', text): text += '\nallow-external-apps = true\n'
        if text != previous:
            backup = root / '.setup-termux-properties.backup'
            if not backup.exists(): backup.write_text(previous)
            settings.write_text(text)
        subprocess.run(['termux-reload-settings'], check=False)
        report('この端末に合うNPUエンジンを準備しています', phase='runtime', command_ready=True, error_code='', free=shutil.disk_usage(root).free)
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
            downloads = root / '.setup-downloads'; downloads.mkdir(exist_ok=True)
            original = downloads / 'model.gguf'
            part = original.with_suffix('.gguf.part')
            control = job.control()
            if control.get('command') == 'reset':
                part.unlink(missing_ok=True); original.unlink(missing_ok=True)
                report('途中の取得データを取り直します', phase='download', ack_reset=True)
            split_folder = downloads / 'split'; split_folder.mkdir(exist_ok=True)
            for name in ['core.gguf','ple.gguf','manifest.json','bolt-template.jinja']:
                (split_folder / name).unlink(missing_ok=True)
            have = original.stat().st_size if original.exists() else (part.stat().st_size if part.exists() else 0)
            required = max(0, MODEL_BYTES-have) + MODEL_BYTES + 2*GIB
            free = shutil.disk_usage(root).free
            report('ダウンロードの準備をしています', phase='download', bytes=have, total=MODEL_BYTES, required=required, free=free)
            if free < required: raise OSError(f'空き容量が不足しています。あと{(required-free)/GIB:.1f} GiB空けて「再開」を押してください。')
            url = source_url(MODEL_URL, control.get('origin', ''))
            fetch(url, original, MODEL_BYTES, MODEL_SHA, event=report, control=job.control)
            report('会話用ファイルを準備しています。品質を変えずにメモリを節約します', phase='split', bytes=0, total=0)
            sys.path.insert(0, str(bundle))
            from split_bolt_ple import split
            split(original, split_folder)
            folder.parent.mkdir(parents=True, exist_ok=True)
            if folder.exists(): folder.rmdir()  # 上で空ディレクトリと確認したものだけ。
            os.replace(split_folder, folder)
            original.unlink()  # インストーラー自身が取得した原本。分離と照合の成功後だけ解放。
        else: report('導入済みの会話モデルを使います', phase='runtime')
        boot = Path.home() / '.termux/boot'; boot.mkdir(parents=True, exist_ok=True)
        boot_script = boot / '30-gemma-atelier'
        boot_script.write_text('#!/data/data/com.termux/files/usr/bin/sh\nexec sh "$HOME/gemma-e4b/boot-atelier.sh"\n')
        boot_script.chmod(0o700)
        env = root / 'atelier-llm.env'
        if not env.exists(): env.write_text('ATELIER_LLM_PROFILE=bolt\nATELIER_LLM_THINKING=off\nATELIER_LLM_FALLBACK=none\nATELIER_IMAGES_OPTIONAL=1\n')
        report('端末のNPUで会話モデルを起動しています', phase='npu')
        subprocess.run(['sh', str(root / 'start-bolt.sh')], check=True, timeout=240)
        verify_npu((root / 'bolt-server.log').read_text(errors='replace'))
        entrypoint = 'restart-after-setup.sh' if bridge_changed else 'start-atelier.sh'
        subprocess.run(['sh', str(root / entrypoint)], check=True, timeout=90)
        status = json.load(urllib.request.urlopen('http://127.0.0.1:8085/api/status', timeout=8))
        if not status.get('gemma'): raise RuntimeError('会話APIの接続を確認できません')
        (root / 'setup-complete.json').write_text(json.dumps({'version': '1.2.1', 'npu': 'HTP0-v75', 'model': 'Bolt-E4B', 'images': status.get('images',False)}))
        report('準備できました。さっそく話してみましょう', 'complete', phase='ready', version=contents['version'], npu=True, images=status.get('images',False), bytes=MODEL_BYTES, total=MODEL_BYTES, free=shutil.disk_usage(root).free)
    except Paused as error:
        report(str(error), 'paused', phase='download', speed=0, eta=-1)
    except Exception as error:
        report(str(error), 'error', error_code='integrity' if isinstance(error, IntegrityError) else 'setup', speed=0, eta=-1)
        raise
    finally: stop.set()


if __name__ == '__main__': main()
