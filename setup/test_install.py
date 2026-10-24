import hashlib
import http.server
import tempfile
import threading
import unittest
from pathlib import Path
from install import fetch, check_device, verify_npu, installed_model
from download import Paused, Job, source_url
import json

DATA = b'bolt download fixture' * 100


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_GET(self):
        start = int(self.headers['Range'][6:-1])
        if self.path == '/full':
            self.send_response(200); start = 0
        else:
            self.send_response(206)
            offset = start + 1 if self.path == '/bad-range' else start
            self.send_header('Content-Range', f'bytes {offset}-{len(DATA)-1}/{len(DATA)}')
        self.end_headers(); self.wfile.write(DATA[start:])


class InstallTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = http.server.HTTPServer(('127.0.0.1', 0), Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True); cls.thread.start()
    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown(); cls.server.server_close(); cls.thread.join()
    def download(self, route, sha=None):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'model.gguf'
            path.with_suffix('.gguf.part').write_bytes(DATA[:17])
            fetch(f'http://127.0.0.1:{self.server.server_port}/{route}', path, len(DATA), sha or hashlib.sha256(DATA).hexdigest())
            self.assertEqual(path.read_bytes(), DATA)
    def test_range_resume(self): self.download('range')
    def test_server_ignoring_range_restarts(self): self.download('full')
    def test_bad_range_rejected(self):
        with self.assertRaises(ValueError): self.download('bad-range')
    def test_bad_hash_rejected(self):
        with self.assertRaises(ValueError): self.download('full', '0'*64)
    def test_supported_and_unsupported_devices(self):
        def props(soc, sdk): return lambda k: soc if k == 'ro.soc.model' else sdk
        check_device(props('SM8650', '35'), 15*1024**3)
        for soc,sdk,ram in [('SM8550','35',15),('SM8650','30',15),('SM8650','35',8)]:
            with self.assertRaises(ValueError): check_device(props(soc,sdk),ram*1024**3)
    def test_npu_proof_is_required(self):
        lines = ['libggml-htp-v75.so', 'offloaded 43/43 layers', 'PLE integrity enabled']
        verify_npu('\n'.join(lines))
        for i in range(3):
            with self.assertRaises(RuntimeError): verify_npu('\n'.join(lines[:i]+lines[i+1:]))
    def test_missing_ple_cannot_be_reused(self):
        with tempfile.TemporaryDirectory() as folder:
            self.assertFalse(installed_model(Path(folder)))
    def test_pause_preserves_partial_and_resume_reports_verification(self):
        with tempfile.TemporaryDirectory() as folder:
            path=Path(folder)/'model.gguf'; part=path.with_suffix('.gguf.part'); part.write_bytes(DATA[:37])
            args=(f'http://127.0.0.1:{self.server.server_port}/range',path,len(DATA),hashlib.sha256(DATA).hexdigest())
            with self.assertRaises(Paused): fetch(*args,control=lambda:{'command':'pause'})
            self.assertEqual(part.read_bytes(),DATA[:37]); self.assertFalse(path.exists())
            events=[];fetch(*args,event=lambda **kw:events.append(kw))
            self.assertEqual(path.read_bytes(),DATA);self.assertFalse(part.exists())
            self.assertIn('download',[e['phase'] for e in events]);self.assertIn('verify',[e['phase'] for e in events])
    def test_job_survives_callback_reconnection_and_phase_has_no_stale_speed(self):
        with tempfile.TemporaryDirectory() as folder:
            first='http://127.0.0.1:1/'+'a'*48;second='http://127.0.0.1:1/'+'b'*48
            a=Job(folder,first);a.report('取得中',phase='download',bytes=12,total=30,speed=20,eta=1)
            b=Job(folder,second);self.assertEqual(a.url(),second);self.assertEqual(b.current['bytes'],12)
            a.report('準備中',phase='split');saved=json.loads((Path(folder)/'.setup-status.json').read_text())
            self.assertEqual(saved['speed'],0);self.assertEqual(saved['total'],0);self.assertEqual(saved['phase'],'split')
    def test_source_preserves_model_path_and_rejects_credentials_or_paths(self):
        self.assertEqual(source_url('https://huggingface.co/a/b/resolve/main/x','https://hf-mirror.com'),'https://hf-mirror.com/a/b/resolve/main/x')
        for origin in ['http://example.com','https://user:pass@example.com','https://example.com/path','https://example.com?q=1']:
            with self.assertRaises(ValueError):source_url('https://huggingface.co/model',origin)


if __name__ == '__main__': unittest.main()
