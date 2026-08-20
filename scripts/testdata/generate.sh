#!/usr/bin/env bash
# Genera el corpus de documentos de prueba en scripts/testdata/out/.
#
# Los formatos que necesitan pandoc (.docx, .epub) se generan dentro del
# contenedor del worker, que ya lo trae instalado; el resto se escribe
# directamente. Requiere el stack levantado para docx/epub.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$HERE/out"
mkdir -p "$OUT"

echo "==> Markdown / texto / HTML"

cat > "$OUT/estructurado.md" <<'DOC'
# Capitulo Primero

Contenido exclusivo del capitulo primero. TOKEN_UNO.

## Seccion Intermedia

Contenido de la seccion intermedia. TOKEN_DOS.

# Capitulo Ultimo

Contenido del capitulo ultimo. TOKEN_TRES.

- primer punto
- segundo punto
DOC

# Caso "documento breve": sin divisiones -> debe producir UN concepto.
cat > "$OUT/breve.md" <<'DOC'
Un documento breve, sin encabezados ni divisiones de ningun tipo.
Debe producir un unico concepto sin advertencias ni errores.
DOC

cat > "$OUT/plano.txt" <<'DOC'
INTRODUCCION

Texto de la introduccion del documento de prueba. TOKEN_UNO.

2. Desarrollo

Texto del desarrollo del documento de prueba. TOKEN_DOS.

CONCLUSIONES

Texto de las conclusiones del documento. TOKEN_TRES.
DOC

cat > "$OUT/pagina.html" <<'DOC'
<!doctype html>
<html lang="es">
  <head><meta charset="utf-8"><title>Documento HTML de prueba</title></head>
  <body>
    <h1>Alfa</h1>
    <p>Cuerpo de la seccion alfa. TOKEN_UNO.</p>
    <h1>Beta</h1>
    <p>Cuerpo de la seccion beta. TOKEN_DOS.</p>
    <h1>Gamma</h1>
    <p>Cuerpo de la seccion gamma. TOKEN_TRES.</p>
  </body>
</html>
DOC

echo "==> Recursos (imagenes) y documento que los referencia"
python "$HERE/make_png.py" "$OUT/diagrama.png" --color 70,90,200 >/dev/null
python "$HERE/make_png.py" "$OUT/grafico.png" --color 200,80,70 --size 24 >/dev/null

cat > "$OUT/con-imagenes.md" <<'DOC'
# Seccion con diagrama

Este concepto referencia una imagen local que debe acabar en assets/.

![Diagrama de arquitectura](diagrama.png)

# Seccion con grafico

Segundo concepto con otro recurso distinto.

![Grafico de resultados](grafico.png)

# Seccion sin recursos

Este concepto no referencia ninguna imagen.
DOC

echo "==> PDF (con y sin marcadores)"
python "$HERE/make_pdf.py" "$OUT/pdf-con-marcadores.pdf" --pages 4 >/dev/null
python "$HERE/make_pdf.py" "$OUT/pdf-sin-marcadores.pdf" --no-outline --pages 3 >/dev/null
# PDF grande: procesamiento prolongado, para evidenciar asincronia y cancelacion.
python "$HERE/make_pdf.py" "$OUT/pdf-grande.pdf" --pages 150 >/dev/null

echo "==> DOCX / EPUB (via pandoc en el worker)"
CID="$(docker compose ps -q worker 2>/dev/null | head -1)"
if [ -z "$CID" ]; then
  echo "    !! worker no está corriendo: se omiten .docx y .epub"
  echo "       levanta el stack con 'docker compose up -d' y repite."
else
  docker cp "$OUT/estructurado.md" "$CID:/tmp/gen.md" >/dev/null
  docker compose exec -T worker sh -c \
    'pandoc /tmp/gen.md -o /tmp/gen.docx && pandoc /tmp/gen.md --metadata title="Documento de prueba" -o /tmp/gen.epub' >/dev/null 2>&1
  docker cp "$CID:/tmp/gen.docx" "$OUT/estructurado.docx" >/dev/null
  docker cp "$CID:/tmp/gen.epub" "$OUT/estructurado.epub" >/dev/null
fi

echo
echo "Documentos generados en $OUT:"
ls -1sh "$OUT"
