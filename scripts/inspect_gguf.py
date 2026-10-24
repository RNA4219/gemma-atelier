"""GGUFのヘッダだけを読み、PLEのファイル範囲を記録する。"""
import argparse
import json
import os
import struct


def inspect(path):
    scalar = {0: "B", 1: "b", 2: "H", 3: "h", 4: "I", 5: "i", 6: "f", 7: "?", 10: "Q", 11: "q", 12: "d"}
    with open(path, "rb") as stream:
        def read(fmt):
            size = struct.calcsize("<" + fmt)
            data = stream.read(size)
            if len(data) != size:
                raise EOFError("GGUFヘッダが不完全")
            return struct.unpack("<" + fmt, data)[0]

        def string(keep=True):
            length = read("Q")
            if keep:
                data = stream.read(length)
                if len(data) != length:
                    raise EOFError("GGUF文字列が不完全")
                return data.decode("utf-8")
            stream.seek(length, 1)

        def value(kind, keep):
            if kind == 8:
                return string(keep)
            if kind == 9:
                element, count = read("I"), read("Q")
                if element in scalar and not keep:
                    stream.seek(struct.calcsize("<" + scalar[element]) * count, 1)
                    return None
                values = [] if keep else None
                for _ in range(count):
                    item = value(element, keep)
                    if keep:
                        values.append(item)
                return values
            if kind not in scalar:
                raise ValueError(f"未知のGGUF型: {kind}")
            result = read(scalar[kind])
            return result if keep else None

        if stream.read(4) != b"GGUF":
            raise ValueError("GGUFではない")
        version, count, metadata_count = read("I"), read("Q"), read("Q")
        metadata = {}
        for _ in range(metadata_count):
            key, kind = string(), read("I")
            keep = key.startswith("general.") or (not key.startswith("tokenizer.") and kind != 9)
            entry = value(kind, keep)
            if keep:
                metadata[key] = entry
        tensors = []
        for _ in range(count):
            name, dimensions = string(), read("I")
            shape = [read("Q") for _ in range(dimensions)]
            tensor_type, offset = read("I"), read("Q")
            tensors.append(dict(name=name, shape=shape, type_id=tensor_type, relative_offset=offset))
        alignment = metadata.get("general.alignment", 32)
        data_offset = (stream.tell() + alignment - 1) // alignment * alignment
        tensors.sort(key=lambda item: item["relative_offset"])
        selected = []
        size = os.path.getsize(path)
        for index, tensor in enumerate(tensors):
            if "embd" not in tensor["name"]:
                continue
            item = dict(tensor)
            item["file_offset"] = data_offset + item["relative_offset"]
            if index + 1 < len(tensors):
                item["aligned_span_bytes"] = tensors[index + 1]["relative_offset"] - item["relative_offset"]
            else:
                item["aligned_span_bytes"] = size - item["file_offset"]
            selected.append(item)
        return dict(path=os.path.abspath(path), version=version, current_file_bytes=size,
                    tensor_count=count, data_offset=data_offset, metadata=metadata, embeddings=selected)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("path")
    parser.add_argument("--output")
    args = parser.parse_args()
    result = json.dumps(inspect(args.path), ensure_ascii=False, indent=2)
    if args.output:
        with open(args.output, "x", encoding="utf-8") as stream:
            stream.write(result + "\n")
    print(result)
