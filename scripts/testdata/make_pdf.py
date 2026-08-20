#!/usr/bin/env python3
"""Genera PDFs de prueba sin dependencias externas (PDF 1.4 a mano).

Se usa para producir casos de prueba deterministas del pipeline de PDF:
  - con marcadores (outline)  -> ejercita el nivel 1 del conversor
  - sin marcadores            -> cae a los niveles 2/3 (heurística / rangos)

Uso:
    python make_pdf.py salida.pdf [--no-outline] [--pages N]
"""
import sys

PAGE_W, PAGE_H = 612, 792


def escape(text):
    return text.replace("\\", r"\\").replace("(", r"\(").replace(")", r"\)")


def page_stream(title, body_lines, title_size=20, body_size=11):
    """Contenido de una página: un título grande y varias líneas de cuerpo.

    El salto de tamaño de fuente (20 vs 11) es lo que permite al nivel 2
    del conversor detectar el título por heurística.
    """
    parts = [
        "BT", f"/F2 {title_size} Tf", f"1 0 0 1 72 {PAGE_H - 90} Tm",
        f"({escape(title)}) Tj", "ET",
    ]
    y = PAGE_H - 130
    for line in body_lines:
        parts += ["BT", f"/F1 {body_size} Tf", f"1 0 0 1 72 {y} Tm",
                  f"({escape(line)}) Tj", "ET"]
        y -= 16
    return "\n".join(parts).encode("latin-1", "replace")


def build(path, sections, with_outline=True):
    objects = {}          # numero -> bytes del cuerpo del objeto
    n_sections = len(sections)

    # 1 catalog, 2 pages, 3 font F1, 4 font F2, luego páginas y contenidos.
    first_page = 5
    page_ids = [first_page + i * 2 for i in range(n_sections)]
    content_ids = [first_page + i * 2 + 1 for i in range(n_sections)]
    outline_root = first_page + n_sections * 2
    outline_items = [outline_root + 1 + i for i in range(n_sections)]

    catalog = f"<< /Type /Catalog /Pages 2 0 R"
    if with_outline:
        catalog += f" /Outlines {outline_root} 0 R /PageMode /UseOutlines"
    catalog += " >>"
    objects[1] = catalog.encode()

    kids = " ".join(f"{p} 0 R" for p in page_ids)
    objects[2] = f"<< /Type /Pages /Count {n_sections} /Kids [{kids}] >>".encode()
    objects[3] = b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
    objects[4] = b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>"

    for i, (title, body) in enumerate(sections):
        stream = page_stream(title, body)
        objects[page_ids[i]] = (
            f"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 {PAGE_W} {PAGE_H}] "
            f"/Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> "
            f"/Contents {content_ids[i]} 0 R >>"
        ).encode()
        objects[content_ids[i]] = (
            f"<< /Length {len(stream)} >>\nstream\n".encode() + stream + b"\nendstream"
        )

    if with_outline:
        objects[outline_root] = (
            f"<< /Type /Outlines /First {outline_items[0]} 0 R "
            f"/Last {outline_items[-1]} 0 R /Count {n_sections} >>"
        ).encode()
        for i, (title, _) in enumerate(sections):
            item = f"<< /Title ({escape(title)}) /Parent {outline_root} 0 R /Dest [{page_ids[i]} 0 R /Fit]"
            if i > 0:
                item += f" /Prev {outline_items[i - 1]} 0 R"
            if i < n_sections - 1:
                item += f" /Next {outline_items[i + 1]} 0 R"
            item += " >>"
            objects[outline_items[i]] = item.encode()

    # Serialización con tabla xref.
    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = {}
    for num in sorted(objects):
        offsets[num] = len(out)
        out += f"{num} 0 obj\n".encode() + objects[num] + b"\nendobj\n"

    xref_pos = len(out)
    max_obj = max(objects) + 1
    out += f"xref\n0 {max_obj}\n".encode()
    out += b"0000000000 65535 f \n"
    for num in range(1, max_obj):
        out += f"{offsets.get(num, 0):010d} 00000 n \n".encode()
    out += (
        f"trailer\n<< /Size {max_obj} /Root 1 0 R >>\nstartxref\n{xref_pos}\n%%EOF\n"
    ).encode()

    with open(path, "wb") as fh:
        fh.write(out)
    return len(out)


def default_sections(pages):
    body = [
        "Este parrafo pertenece exclusivamente a esta seccion del documento",
        "y sirve para verificar que la segmentacion no arrastra contenido",
        "de una unidad logica a la siguiente.",
    ]
    return [
        (f"Capitulo {i + 1}. Unidad logica numero {i + 1}",
         body + [f"Marcador unico de la seccion: TOKEN_{i + 1}."])
        for i in range(pages)
    ]


if __name__ == "__main__":
    args = sys.argv[1:]
    if not args:
        print(__doc__)
        sys.exit(1)
    out_path = args[0]
    with_outline = "--no-outline" not in args
    pages = 4
    if "--pages" in args:
        pages = int(args[args.index("--pages") + 1])
    size = build(out_path, default_sections(pages), with_outline)
    print(f"{out_path}: {size} bytes, {pages} secciones, outline={with_outline}")
