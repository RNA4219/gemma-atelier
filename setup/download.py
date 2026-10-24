"""中断・再開と、取得と検証を区別したダウンロード処理。"""
import hashlib
import json
import os
from pathlib import Path
import re
import time
import threading
import urllib.request
import urllib.parse
from urllib.error import HTTPError, URLError


class Paused(Exception): pass
class IntegrityError(ValueError): pass


def source_url(url, origin):
    if not origin: return url
    p = urllib.parse.urlsplit(origin)
    if p.scheme != 'https' or not p.hostname or p.username or p.password or p.path not in ['', '/'] or p.query or p.fragment:
        raise ValueError('取得元は https://example.com の形式で指定してください')
    return origin.rstrip('/') + urllib.parse.urlsplit(url).path


def fetch(url, path, expected_size, expected_sha, progress=lambda n: None, *, event=lambda **kw: None, control=lambda: {}):
    path = Path(path); part = path.with_suffix(path.suffix + '.part')
    def paused():
        c = control()
        if c.get('command') == 'pause': raise Paused(c.get('reason') or '途中までのデータを保存しました')
    def verify(p):
        h = hashlib.sha256(); n = 0; last = 0
        with p.open('rb') as stream:
            for block in iter(lambda: stream.read(4 << 20), b''):
                h.update(block); n += len(block)
                if time.monotonic() - last > 1:
                    event(phase='verify', message='ダウンロードしたファイルを確認しています', bytes=n, total=expected_size, speed=0, eta=-1)
                    last = time.monotonic()
        if n != expected_size or h.hexdigest() != expected_sha:
            raise IntegrityError('ファイルの確認に失敗しました。「取り直す」で途中データだけを消して再取得できます。')
    if path.exists(): verify(path); return
    started = time.monotonic(); initial = part.stat().st_size if part.exists() else 0
    n = initial
    if initial > expected_size: raise IntegrityError('途中ファイルのサイズが不正です。「取り直す」を選んでください。')
    for attempt in range(4):
        paused(); n = part.stat().st_size if part.exists() else 0
        if n == expected_size: break
        event(phase='download', message='会話モデルをダウンロードしています', bytes=n, total=expected_size, speed=0, eta=-1)
        try:
            req = urllib.request.Request(url, headers={'Range': f'bytes={n}-', 'User-Agent': 'LLM-Atelier/1.2.1'})
            with urllib.request.urlopen(req, timeout=20) as response:
                if response.status == 206:
                    match = re.fullmatch(r'bytes (\d+)-(\d+)/(\d+)', response.headers.get('Content-Range', ''))
                    if not match or int(match[1]) != n or int(match[3]) != expected_size or int(match[2]) < n or int(match[2]) >= expected_size:
                        raise IntegrityError('ダウンロード元の再開位置が一致しません')
                    mode = 'ab'
                elif response.status == 200:
                    n = 0; initial = 0; started = time.monotonic(); mode = 'wb'
                else: raise OSError('取得元が正しいデータを返しませんでした')
                last = 0
                with part.open(mode) as out:
                    while True:
                        if time.monotonic() - last >= .75:
                            paused(); speed = max(0, (n - initial) / max(.1, time.monotonic() - started))
                            event(phase='download', message='会話モデルをダウンロードしています', bytes=n, total=expected_size,
                                  speed=round(speed), eta=round((expected_size-n)/speed) if speed > 0 else -1)
                            progress(n); last = time.monotonic()
                        block = response.read(1 << 20)
                        if not block: break
                        if n + len(block) > expected_size: raise IntegrityError('予定したサイズを超える応答を停止しました')
                        out.write(block); n += len(block)
            if n == expected_size: break
            raise OSError('通信が途中で切れました')
        except (OSError, HTTPError, URLError) as error:
            if attempt == 3: raise OSError('通信を確認して「再開」を押してください。途中のデータは保持しています。') from error
            event(phase='waiting', message=f'通信を再確認しています（{attempt+1}/3）', bytes=n, total=expected_size, speed=0, eta=-1)
            for _ in range(2 + attempt * 2): paused(); time.sleep(1)
    verify(part)
    os.replace(part, path)
    progress(expected_size)


class Job:
    def __init__(self, root, callback):
        self.root = Path(root); self.status = self.root / '.setup-status.json'; self.callback = self.root / '.setup-callback'
        if not re.fullmatch(r'http://127\.0\.0\.1:\d+/[0-9a-f]{48}', callback): raise ValueError('準備用の接続先が不正です')
        self.root.mkdir(exist_ok=True)
        temp = self.callback.with_suffix(f'.{os.getpid()}.new')
        temp.write_text(callback, encoding='ascii'); os.replace(temp, self.callback)
        self.current = {}
        self.mutex = threading.RLock()
        if self.status.exists():
            try: self.current = json.loads(self.status.read_text())
            except (ValueError, OSError): pass
    def url(self):
        value = self.callback.read_text().strip()
        if not re.fullmatch(r'http://127\.0\.0\.1:\d+/[0-9a-f]{48}', value): raise ValueError('接続先が不正です')
        return value
    def control(self):
        try:
            with urllib.request.urlopen(self.url() + '/control', timeout=2) as r: return json.load(r)
        except (OSError, ValueError):
            return {'command':'pause','reason':'管理画面との接続が切れました。画面を開いて「再開」を押してください。'}
    def report(self, message=None, state='running', **fields):
        with self.mutex: self._report(message, state, **fields)
    def pulse(self):
        with self.mutex:
            if self.current.get('state') == 'running': self._report(None, 'running')
    def _report(self, message, state, **fields):
        if fields.get('phase') and fields['phase'] != self.current.get('phase'):
            self.current.update(bytes=0, total=0, speed=0, eta=-1)
        self.current.update(fields, state=state, updated_at=time.time(), job='bolt')
        if message is not None: self.current['message'] = message
        data = json.dumps(self.current, ensure_ascii=False).encode()
        temp = self.status.with_suffix('.new'); temp.write_bytes(data); os.replace(temp, self.status)
        try:
            req = urllib.request.Request(self.url()+'/progress', data=data, headers={'Content-Type':'application/json'})
            urllib.request.urlopen(req, timeout=3).close()
        except (OSError, ValueError): pass
        self.current.pop('ack_reset', None)
