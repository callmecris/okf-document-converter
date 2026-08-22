#!/usr/bin/env bash
# Prueba de extremo a extremo de las condiciones verificables (spec §6).
# Requiere el stack levantado y los documentos de scripts/testdata/out/.
#
#   bash scripts/e2e.sh [http://localhost:8080]
set -uo pipefail

API="${1:-http://localhost:${API_PORT:-8080}}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCS="$HERE/testdata/out"
PASS=0; FAIL=0

ok()   { echo "  [OK]   $1"; PASS=$((PASS+1)); }
bad()  { echo "  [FALLA] $1"; FAIL=$((FAIL+1)); }
check(){ if [ "$2" = "$3" ]; then ok "$1 ($3)"; else bad "$1: esperado '$3', obtenido '$2'"; fi; }

jqr() { python -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }

if [ ! -d "$DOCS" ]; then
  echo "!! Faltan documentos de prueba. Ejecuta: bash scripts/testdata/generate.sh"
  exit 1
fi

echo "== API: $API =="
STAMP=$(date +%s)
USER_A="a-$STAMP@okf.test"; USER_B="b-$STAMP@okf.test"

TOK_A=$(curl -s -X POST "$API/api/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$USER_A\",\"password\":\"password123\"}" | jqr "d['token']")
TOK_B=$(curl -s -X POST "$API/api/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$USER_B\",\"password\":\"password123\"}" | jqr "d['token']")
[ -n "$TOK_A" ] && ok "registro y autenticacion" || { bad "registro"; exit 1; }

upload() { curl -s -X POST "$API/api/v1/jobs" -H "Authorization: Bearer $TOK_A" -F "file=@$1" ; }
status() { curl -s -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$1" ; }

wait_job() {
  for _ in $(seq 1 40); do
    st=$(status "$1" | jqr "d['status']")
    case "$st" in completed|failed) echo "$st"; return;; esac
    sleep 1
  done
  echo timeout
}

echo
echo "== 1. Asincronia: la carga responde de inmediato =="
RESP=$(upload "$DOCS/pdf-con-marcadores.pdf")
JOB_PDF=$(echo "$RESP" | jqr "d['id']")
check "estado inicial devuelto sin esperar la conversion" "$(echo "$RESP" | jqr "d['status']")" "pending"

echo
echo "== 2. Conversion de todos los formatos =="
declare -A JOBS
for f in estructurado.md breve.md plano.txt pagina.html estructurado.docx estructurado.epub pdf-sin-marcadores.pdf; do
  [ -f "$DOCS/$f" ] || { echo "  (omitido $f: no existe)"; continue; }
  JOBS[$f]=$(upload "$DOCS/$f" | jqr "d['id']")
done
JOBS[pdf-con-marcadores.pdf]=$JOB_PDF

for f in "${!JOBS[@]}"; do
  st=$(wait_job "${JOBS[$f]}")
  check "$f" "$st" "completed"
done

echo
echo "== 3. Documento breve: un unico concepto =="
BREVE=$(status "${JOBS[breve.md]}")
N=$(echo "$BREVE" | jqr "len([f for f in d['bundle']['files'] if '/conceptos/' in f['path']])")
check "conceptos generados" "$N" "1"
check "index.md y log.md presentes" \
  "$(echo "$BREVE" | jqr "sum(1 for f in d['bundle']['files'] if f['path'].endswith(('index.md','log.md')))")" "2"

echo
echo "== 4. Documento estructurado: un concepto por unidad, en orden =="
EST=$(status "${JOBS[estructurado.md]}")
check "conceptos del documento estructurado" \
  "$(echo "$EST" | jqr "len([f for f in d['bundle']['files'] if '/conceptos/' in f['path']])")" "3"
IDX=$(curl -s -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/${JOBS[estructurado.md]}/bundle/index.md")
if echo "$IDX" | grep -q "fragmento-01.md" && echo "$IDX" | grep -q "fragmento-03.md"; then
  ok "index.md enlaza los conceptos en orden"
else
  bad "index.md no enlaza todos los conceptos"
fi

echo
echo "== 5. Sin contaminacion entre conceptos =="
C1=$(curl -s -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/${JOBS[estructurado.md]}/bundle/conceptos/fragmento-01.md")
C2=$(curl -s -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/${JOBS[estructurado.md]}/bundle/conceptos/fragmento-02.md")
echo "$C1" | grep -q TOKEN_UNO && ok "concepto 1 tiene su contenido" || bad "concepto 1 sin su contenido"
if echo "$C2" | grep -q TOKEN_UNO; then bad "concepto 2 arrastra el contenido del 1"; else ok "concepto 2 no arrastra contenido del 1"; fi

echo
echo "== 6. Validacion y clasificacion del resultado =="
VAL=$(echo "$EST" | jqr "d['bundle']['validation']")
case "$VAL" in
  valid|valid_with_warnings) ok "bundle clasificado como '$VAL'";;
  *) bad "clasificacion inesperada: '$VAL'";;
esac

echo
echo "== 7. Aislamiento multiusuario =="
check "GET job ajeno"      "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOK_B" "$API/api/v1/jobs/$JOB_PDF")" "403"
check "GET download ajeno" "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOK_B" "$API/api/v1/jobs/$JOB_PDF/download")" "403"
check "GET archivo ajeno"  "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOK_B" "$API/api/v1/jobs/$JOB_PDF/bundle/index.md")" "403"
check "sin token"          "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/v1/jobs/$JOB_PDF")" "401"
check "retry ajeno"        "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $TOK_B" "$API/api/v1/jobs/$JOB_PDF/retry")" "403"

echo
echo "== 8. Descarga del bundle completo (.zip) =="
ZIP=$(mktemp -u).zip
CODE=$(curl -s -o "$ZIP" -w '%{http_code}' -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$JOB_PDF/download")
check "descarga zip" "$CODE" "200"
ZIP="$ZIP" python -c "
import zipfile,os,sys
z=zipfile.ZipFile(os.environ['ZIP']); n=z.namelist()
assert z.testzip() is None and 'index.md' in n and 'log.md' in n, n
print('  [OK]   zip integro con index.md y log.md (%d entradas)' % len(n))
" || bad "zip invalido"
rm -f "$ZIP"

echo
echo "== 9. Rechazo de formato no soportado =="
BADF=$(mktemp -u).exe; echo x > "$BADF"
check "upload .exe" "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/v1/jobs" -H "Authorization: Bearer $TOK_A" -F "file=@$BADF")" "400"
rm -f "$BADF"

echo
echo "== 10. Reintento: solo aplica a trabajos fallidos =="
check "retry de job completado" "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$JOB_PDF/retry")" "409"

echo
echo "== 11. Assets: recursos extraidos a assets/ =="
if [ -f "$DOCS/con-imagenes.docx" ]; then
  JIMG=$(upload "$DOCS/con-imagenes.docx" | jqr "d['id']")
  check "conversion con imagenes" "$(wait_job "$JIMG")" "completed"
  DET=$(status "$JIMG")
  NASSETS=$(echo "$DET" | jqr "len([f for f in d['bundle']['files'] if '/assets/' in f['path']])")
  if [ "${NASSETS:-0}" -ge 1 ]; then ok "recursos en assets/ ($NASSETS)"; else bad "no se extrajo ningun recurso"; fi
  check "bundle sin advertencias de recursos" "$(echo "$DET" | jqr "d['bundle']['validation']")" "valid"

  ASSET=$(echo "$DET" | jqr "[f['url'] for f in d['bundle']['files'] if '/assets/' in f['path']][0]")
  CT=$(curl -s -o /dev/null -w '%{content_type}' -H "Authorization: Bearer $TOK_A" "$API$ASSET")
  case "$CT" in image/*) ok "el recurso se sirve como $CT";; *) bad "content-type del recurso: $CT";; esac

  C1=$(curl -s -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$JIMG/bundle/conceptos/fragmento-01.md")
  echo "$C1" | grep -q "\.\./assets/" && ok "el concepto referencia ../assets/" || bad "el concepto no referencia assets/"
else
  echo "  (omitido: falta con-imagenes.docx)"
fi

echo
echo "== 12. Cancelacion de trabajos =="
# Documento de procesamiento prolongado: da margen a cancelar en vuelo.
CANCEL_DOC="$DOCS/pdf-grande.pdf"
[ -f "$CANCEL_DOC" ] || CANCEL_DOC="$DOCS/pdf-con-marcadores.pdf"
JC=$(upload "$CANCEL_DOC" | jqr "d['id']")
CRESP=$(curl -s -X POST -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$JC/cancel")
CSTATUS=$(echo "$CRESP" | jqr "d['status']")
if [ "$CSTATUS" = "canceled" ]; then
  ok "cancelacion aceptada (canceled)"
  sleep 4
  check "sigue cancelado tras procesar" "$(status "$JC" | jqr "d['status']")" "canceled"
  check "descarga denegada"    "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$JC/download")" "409"
  check "reintento de cancelado" "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$JC/retry")" "201"
else
  # El worker pudo terminarlo antes de que llegara la cancelacion: no es un
  # fallo del sistema, pero se reporta para que no pase inadvertido.
  echo "  (!) el trabajo termino antes de cancelarse: estado '$CSTATUS'"
fi
check "cancelar job terminado" "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $TOK_A" "$API/api/v1/jobs/$JOB_PDF/cancel")" "409"
check "cancelar job ajeno"     "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $TOK_B" "$API/api/v1/jobs/$JOB_PDF/cancel")" "403"

echo
echo "== 13. Metricas y observabilidad =="
check "GET /metrics (Prometheus)" "$(curl -s -o /dev/null -w '%{http_code}' "$API/metrics")" "200"
curl -s "$API/metrics" | grep -q "okf_jobs_total" && ok "expone okf_jobs_total" || bad "sin okf_jobs_total"
M=$(curl -s "$API/api/v1/metrics")
echo "$M" | jqr "d['total_jobs']" | grep -qE '^[0-9]+$' && ok "JSON con total_jobs ($(echo "$M" | jqr "d['total_jobs']"))" || bad "JSON de metricas invalido"
echo "$M" | jqr "d['jobs_by_status']" >/dev/null && ok "desglose por estado presente" || bad "sin desglose por estado"

echo
echo "======================================"
echo "  PASS: $PASS    FALLA: $FAIL"
echo "======================================"
[ "$FAIL" -eq 0 ]
