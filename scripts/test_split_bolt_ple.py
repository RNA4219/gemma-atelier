import hashlib
import json
from pathlib import Path
import struct
import tempfile
import unittest

from split_bolt_ple import Header, HASH_KEY, PLE, digest_file, kv, pack_string, split, tensor_digest, align


class SplitTests(unittest.TestCase):
    def make_source(self, path):
        meta = [kv('general.architecture', 8, pack_string('gemma4')),
                kv('tokenizer.chat_template', 8, pack_string('Bolt {{ messages }}'))]
        tensors = [('small.weight', [4], 0, 0), (PLE, [32, 3], 8, 32)]
        descriptors = [pack_string(n) + struct.pack('<I', len(s)) + struct.pack('<'+'Q'*len(s), *s) + struct.pack('<IQ', t, o) for n,s,t,o in tensors]
        h = b'GGUF'+struct.pack('<IQQ', 3, 2, len(meta))+b''.join(meta)+b''.join(descriptors)
        data = struct.pack('<4f', 1, 2, 3, 4)+bytes(16)+bytes(range(102))
        path.write_bytes(h+bytes(align(len(h),32)-len(h))+data)

    def test_exact_bytes_metadata_and_row_hashes(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'model.gguf'
            self.make_source(p)
            original=Header(p)
            out=Path(d)/'split'
            m=split(p,out,expected=digest_file(p))
            self.assertFalse(m['quantization_changed'])
            core,ple=Header(out/'core.gguf'),Header(out/'ple.gguf')
            self.assertEqual(core.metadata['tokenizer.chat_template'], original.metadata['tokenizer.chat_template'])
            self.assertEqual([t['name'] for t in ple.tensors],[PLE])
            self.assertEqual(len(core.tensors),1)
            for file, h in [(out/'core.gguf',core),(out/'ple.gguf',ple)]:
                for t in h.tensors:
                    src=next(x for x in original.tensors if x['name']==t['name'])
                    self.assertEqual((t['shape'],t['type_id']),(src['shape'],src['type_id']))
                    self.assertEqual(tensor_digest(file,t),tensor_digest(p,src))
            expected=b''.join(hashlib.sha256(bytes(range(102))[i:i+34]).digest() for i in range(0,102,34))
            self.assertEqual(core.metadata[HASH_KEY][-96:],expected)
            self.assertEqual(json.loads((out/'manifest.json').read_text())['tensor_count'],2)

    def test_bad_source_hash_and_truncation(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'model.gguf'
            self.make_source(p)
            with self.assertRaisesRegex(ValueError,'SHA256'):
                split(p,Path(d)/'split',expected='0'*64)
            p.write_bytes(p.read_bytes()[:-1])
            with self.assertRaisesRegex(ValueError,'truncated tensor'):
                Header(p)


if __name__=='__main__':
    unittest.main()
