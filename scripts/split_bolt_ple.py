"""GGUFの重みを再量子化せず分離し、PLEの行単位SHA256を本体メタデータへ保存する。"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
from pathlib import Path
import struct
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path[:0] = [str(ROOT / 'engine/gguf-py'), str(ROOT / 'converter-deps')]
from gguf.constants import GGML_QUANT_SIZES, GGMLQuantizationType

PLE = 'per_layer_token_embd.weight'
HASH_KEY = 'atelier.ple.row_sha256'
REQUIRED_KEY = 'atelier.ple.required'
SOURCE_SHA = '5442bfb3b1384ff3774c9989e9457dad81bbb5830fbcf96adfdde40078831d5a'
SCALAR = {0:'B', 1:'b', 2:'H', 3:'h', 4:'I', 5:'i', 6:'f', 7:'?', 10:'Q', 11:'q', 12:'d'}


def pack_string(value):
    raw = value.encode('utf-8')
    return struct.pack('<Q', len(raw)) + raw


def kv(key, kind, value):
    return pack_string(key) + struct.pack('<I', kind) + value


def align(value, alignment):
    return (value + alignment - 1) // alignment * alignment


class Header:
    def __init__(self, path, complete=True):
        self.path = Path(path)
        size = self.path.stat().st_size
        self.metadata = {}
        self.values = {}
        self.tensors = []
        with self.path.open('rb') as f:
            def read(n):
                if n < 0 or n > 256 << 20 or f.tell() + n > min(size, 256 << 20):
                    raise ValueError('GGUF header length is out of bounds')
                data = f.read(n)
                if len(data) != n:
                    raise ValueError('truncated GGUF header')
                return data

            def number(fmt):
                return struct.unpack('<' + fmt, read(struct.calcsize('<' + fmt)))[0]

            def string():
                return read(number('Q')).decode('utf-8')

            def value(kind, keep=False):
                if kind == 8:
                    return string()
                if kind in SCALAR:
                    return number(SCALAR[kind])
                if kind != 9:
                    raise ValueError('unsupported metadata type')
                element, count = number('I'), number('Q')
                if count > 16 << 20 or element == 9:
                    raise ValueError('invalid GGUF array')
                if element in SCALAR:
                    read(count * struct.calcsize('<' + SCALAR[element]))
                elif element == 8:
                    for _ in range(count):
                        string()
                else:
                    raise ValueError('invalid array element type')
                return None

            if read(4) != b'GGUF' or number('I') != 3:
                raise ValueError('GGUF version 3 is required')
            nt, nk = number('Q'), number('Q')
            if nt > 100000 or nk > 100000:
                raise ValueError('excessive GGUF item count')
            for _ in range(nk):
                start = f.tell()
                name, kind = string(), number('I')
                if name in self.metadata:
                    raise ValueError('duplicate metadata key: ' + name)
                self.values[name] = value(kind)
                end = f.tell()
                f.seek(start)
                self.metadata[name] = read(end - start)
            names = set()
            for _ in range(nt):
                name, nd = string(), number('I')
                if name in names or not 1 <= nd <= 4:
                    raise ValueError('duplicate tensor or invalid dimension count')
                names.add(name)
                shape = [number('Q') for _ in range(nd)]
                kind, offset = number('I'), number('Q')
                if any(x < 1 or x > 1 << 32 for x in shape):
                    raise ValueError('invalid tensor dimensions')
                block, unit = GGML_QUANT_SIZES[GGMLQuantizationType(kind)]
                if shape[0] % block:
                    raise ValueError('unaligned quantized row')
                nbytes = math.prod(shape) // block * unit
                if nbytes > 1 << 40:
                    raise ValueError('tensor exceeds supported size')
                self.tensors.append(dict(name=name, shape=shape, type_id=kind,
                                         dtype=GGMLQuantizationType(kind).name,
                                         offset=offset, bytes=nbytes))
            self.alignment = self.values.get('general.alignment', 32)
            if not isinstance(self.alignment, int) or self.alignment < 1 or self.alignment > 4096 or self.alignment & (self.alignment - 1):
                raise ValueError('invalid GGUF alignment')
            self.data_offset = align(f.tell(), self.alignment)
            end = self.data_offset
            for t in sorted(self.tensors, key=lambda x: x['offset']):
                t['file_offset'] = self.data_offset + t['offset']
                if t['offset'] % self.alignment or t['file_offset'] < end:
                    raise ValueError('overlapping or unaligned tensor')
                end = t['file_offset'] + t['bytes']
                if complete and end > size:
                    raise ValueError('truncated tensor: ' + t['name'])


def digest_file(path):
    with Path(path).open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()


def tensor_digest(path, tensor):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        f.seek(tensor['file_offset'])
        left = tensor['bytes']
        while left:
            data = f.read(min(left, 8 << 20))
            if not data:
                raise ValueError('truncated tensor')
            h.update(data)
            left -= len(data)
    return h.hexdigest()


def write_shard(source, path, tensors, metadata, shard, total):
    raw_meta = [v for k, v in metadata.items() if not k.startswith('split.')]
    raw_meta.extend([kv('split.no', 2, struct.pack('<H', shard)),
                     kv('split.count', 2, struct.pack('<H', 2)),
                     kv('split.tensors.count', 5, struct.pack('<i', total))])
    descriptors = []
    offsets = []
    offset = 0
    for t in tensors:
        offsets.append(offset)
        descriptors.append(pack_string(t['name']) + struct.pack('<I', len(t['shape'])) +
                           struct.pack('<' + 'Q' * len(t['shape']), *t['shape']) +
                           struct.pack('<IQ', t['type_id'], offset))
        offset = align(offset + t['bytes'], source.alignment)
    header = b'GGUF' + struct.pack('<IQQ', 3, len(tensors), len(raw_meta)) + b''.join(raw_meta) + b''.join(descriptors)
    data_start = align(len(header), source.alignment)
    hashes = {}
    with path.open('xb') as out, source.path.open('rb') as inp:
        out.write(header)
        out.write(bytes(data_start - len(header)))
        for t, offset in zip(tensors, offsets):
            out.write(bytes(data_start + offset - out.tell()))
            inp.seek(t['file_offset'])
            h = hashlib.sha256()
            left = t['bytes']
            while left:
                data = inp.read(min(left, 8 << 20))
                if not data:
                    raise ValueError('source tensor truncated')
                out.write(data)
                h.update(data)
                left -= len(data)
            hashes[t['name']] = h.hexdigest()
    return hashes


def split(path, destination, expected=SOURCE_SHA):
    source = Header(path)
    if source.values.get('general.architecture') != 'gemma4':
        raise ValueError('expected Gemma4 architecture')
    if any(k.startswith('split.') or k in [HASH_KEY, REQUIRED_KEY] for k in source.metadata):
        raise ValueError('expected original unsplit GGUF')
    source_sha = digest_file(path)
    if source_sha != expected:
        raise ValueError('original model SHA256 mismatch')
    ple = next((t for t in source.tensors if t['name'] == PLE), None)
    if ple is None:
        raise ValueError('required PLE tensor is missing')
    if len(ple['shape']) != 2:
        raise ValueError('PLE must be a matrix')
    row_bytes = ple['bytes'] // ple['shape'][1]
    row_hashes = bytearray()
    with source.path.open('rb') as f:
        f.seek(ple['file_offset'])
        for _ in range(ple['shape'][1]):
            row = f.read(row_bytes)
            if len(row) != row_bytes:
                raise ValueError('PLE row truncated')
            row_hashes.extend(hashlib.sha256(row).digest())
    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    for name in ['core.gguf', 'ple.gguf', 'manifest.json']:
        if (destination / name).exists():
            raise FileExistsError(destination / name)
    main_metadata = dict(source.metadata)
    main_metadata[REQUIRED_KEY] = kv(REQUIRED_KEY, 7, b'\1')
    main_metadata[HASH_KEY] = kv(HASH_KEY, 9, struct.pack('<IQ', 0, len(row_hashes)) + row_hashes)
    ple_metadata = {k:v for k,v in source.metadata.items() if k in ['general.architecture', 'general.alignment']}
    groups = [[t for t in source.tensors if t['name'] != PLE], [ple]]
    records = []
    for index, (name, tensors, metadata) in enumerate(zip(['core.gguf', 'ple.gguf'], groups, [main_metadata, ple_metadata])):
        out = destination / name
        hashes = write_shard(source, out, tensors, metadata, index, len(source.tensors))
        check = Header(out)
        for t in check.tensors:
            if tensor_digest(out, t) != hashes[t['name']]:
                raise ValueError('copied tensor SHA256 mismatch: ' + t['name'])
        records.append(dict(file=name, bytes=out.stat().st_size, sha256=digest_file(out),
                            tensors=[dict(t, sha256=hashes[t['name']]) for t in check.tensors]))
        print(json.dumps({k:v for k,v in records[-1].items() if k != 'tensors'}), flush=True)
    core = Header(destination / 'core.gguf')
    for key, data in source.metadata.items():
        if core.metadata[key] != data:
            raise ValueError('metadata changed: ' + key)
    template = source.values.get('tokenizer.chat_template', '')
    manifest = dict(format=1, source_repo='nvidia/NVIDIA-Nemotron-3.5-Bolt-E4B-4GB',
                    revision='4008514238a0b4ae17af3988818f75bf084541a5', source_sha256=source_sha,
                    source_bytes=source.path.stat().st_size, source_file=source.path.name,
                    architecture=source.values['general.architecture'], tensor_count=len(source.tensors),
                    quantization_changed=False, files=records,
                    chat_template_sha256=hashlib.sha256(template.encode()).hexdigest(),
                    tokenizer_metadata_sha256=hashlib.sha256(b''.join(v for k,v in source.metadata.items() if k.startswith('tokenizer.'))).hexdigest(),
                    ple=dict(name=PLE, shape=ple['shape'], dtype=ple['dtype'], bytes=ple['bytes'], row_bytes=row_bytes,
                             integrity='SHA256 per accessed row; digests embedded in core GGUF metadata',
                             row_hashes_sha256=hashlib.sha256(row_hashes).hexdigest()),
                    runtime=dict(load_mode='mmap', lazy_mode='on', device='HTP0', ngl=99, fit='off',
                                 model_splits=['core.gguf','ple.gguf'], storage='nubia internal storage, not NVMe'))
    (destination / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    (destination / 'bolt-template.jinja').write_text(template, encoding='utf-8')
    return manifest


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--inspect', action='store_true')
    args = parser.parse_args()
    if args.inspect:
        h = Header(args.source, complete=False)
        print(json.dumps(dict(tensors=len(h.tensors), ple=[t for t in h.tensors if t['name']==PLE],
                              template_chars=len(h.values.get('tokenizer.chat_template',''))), indent=2))
    else:
        if args.output is None:
            parser.error('--output is required for splitting')
        split(args.source, args.output)
