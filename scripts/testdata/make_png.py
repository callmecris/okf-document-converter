#!/usr/bin/env python3
"""Genera un PNG minimo valido (sin dependencias) para probar assets/.

Uso: python make_png.py salida.png [--color R,G,B] [--size N]
"""
import struct
import sys
import zlib


def png(path, size=16, color=(70, 90, 200)):
    raw = b""
    for _ in range(size):
        raw += b"\x00" + bytes(color) * size  # filtro 0 + fila RGB

    def chunk(tag, data):
        body = tag + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body))

    ihdr = struct.pack(">IIBBBBB", size, size, 8, 2, 0, 0, 0)  # 8-bit RGB
    out = (b"\x89PNG\r\n\x1a\n"
           + chunk(b"IHDR", ihdr)
           + chunk(b"IDAT", zlib.compress(raw))
           + chunk(b"IEND", b""))
    with open(path, "wb") as fh:
        fh.write(out)
    return len(out)


if __name__ == "__main__":
    args = sys.argv[1:]
    if not args:
        print(__doc__)
        sys.exit(1)
    size, color = 16, (70, 90, 200)
    if "--size" in args:
        size = int(args[args.index("--size") + 1])
    if "--color" in args:
        color = tuple(int(x) for x in args[args.index("--color") + 1].split(","))
    print(f"{args[0]}: {png(args[0], size, color)} bytes")
