"""確認済みのARM64/Hexagon成果物を、初回セットアップAPK用に梱包する。"""
import argparse
import hashlib
import io
import json
from pathlib import Path
import tarfile
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--server', type=Path, required=True)
    p.add_argument('--dsp', type=Path, required=True)
    p.add_argument('--bridge', type=Path, required=True)
    p.add_argument('--engine', type=Path, required=True)
    args = p.parse_args()
    payload = {
        'bin/llama-server-bolt': args.server,
        'bin/libggml-htp-v75.so': args.dsp,
        'bin/chat-bridge-bolt': args.bridge,
        'engine-env.sh': ROOT / 'scripts/termux-engine-env.sh',
    }
    for name in ['start-bolt.sh', 'stop-bolt.sh', 'stop-chat.sh', 'start-chat.sh',
                 'start-atelier.sh', 'start-atelier-profile.sh', 'start-from-home.sh',
                 'boot-atelier.sh', 'restart-after-setup.sh']:
        payload[name] = ROOT / 'scripts' / name
    assets = ROOT / 'launcher/assets'; assets.mkdir(exist_ok=True)
    manifest = {'version': '1.2.1', 'engine_commit': 'd454a204ef2b63af357744809c05af897611a72e',
                'soc': 'SM8650', 'hexagon': 'v75', 'files': []}
    files = {}
    for name, source in payload.items():
        data = source.read_bytes()
        if source.suffix == '.sh': data = data.replace(b'\r\n', b'\n')
        executable = source.suffix in {'.sh', '.so'} or name.startswith('bin/')
        manifest['files'].append({'path': name, 'bytes': len(data), 'sha256': hashlib.sha256(data).hexdigest(), 'executable': executable})
        files['payload/' + name] = data
    for name, path in {
        'install.py': ROOT / 'setup/install.py',
        'download.py': ROOT / 'setup/download.py',
        'split_bolt_ple.py': ROOT / 'scripts/split_bolt_ple.py',
        'gguf/constants.py': args.engine / 'gguf-py/gguf/constants.py',
        'licenses/llama.cpp.txt': args.engine / 'LICENSE',
    }.items(): files[name] = path.read_bytes().replace(b'\r\n', b'\n')
    files['gguf/__init__.py'] = b''
    go_root = Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())
    files['licenses/Go.txt'] = (go_root / 'LICENSE').read_bytes()
    files['manifest.json'] = json.dumps(manifest, ensure_ascii=False, indent=2).encode()
    output = assets / 'runtime.tar'
    with tarfile.open(output, 'w', format=tarfile.USTAR_FORMAT) as tar:
        for name, data in sorted(files.items()):
            entry = tarfile.TarInfo(name); entry.size = len(data); entry.mode = 0o600
            tar.addfile(entry, io.BytesIO(data))
    sha = hashlib.sha256(output.read_bytes()).hexdigest()
    (assets / 'runtime.sha256').write_text(sha + '\n', encoding='ascii', newline='\n')
    (assets / 'bootstrap.sh').write_bytes((ROOT / 'setup/bootstrap.sh').read_bytes().replace(b'\r\n', b'\n'))
    (assets / 'runtime-manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n', encoding='utf-8', newline='\n')
    print(json.dumps({'runtime_bytes': output.stat().st_size, 'sha256': sha}))


if __name__ == '__main__': main()
