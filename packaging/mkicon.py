#!/usr/bin/env python3
"""Write a 256x256 PNG icon. No third-party modules."""
import struct
import sys
import zlib


def chunk(tag, data):
    return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)


def main(path):
    w = h = 256
    raw = bytearray()
    for y in range(h):
        raw.append(0)
        for x in range(w):
            # Dark panel with a green bar, matching the editor theme.
            if 28 <= x <= 44 and 48 <= y <= 208:
                raw.extend((98, 150, 85, 255))
            elif 56 <= x <= 200 and 72 <= y <= 96:
                raw.extend((188, 190, 196, 255))
            elif 56 <= x <= 160 and 112 <= y <= 128:
                raw.extend((106, 140, 196, 255))
            elif 56 <= x <= 180 and 144 <= y <= 160:
                raw.extend((188, 190, 196, 255))
            else:
                raw.extend((30, 31, 34, 255))
    ihdr = struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(bytes(raw), 9)) + chunk(b"IEND", b"")
    with open(path, "wb") as f:
        f.write(png)


if __name__ == "__main__":
    main(sys.argv[1])
