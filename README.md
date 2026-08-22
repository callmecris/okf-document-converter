# OKF — Conversión de Documentos (PDF, DOCX, EPUB) a Bundles Markdown

Arquitectura Cloud asíncrona: la API encola trabajos en RabbitMQ y los
**workers** los procesan de forma aislada (cada worker procesa un documento
en un directorio temporal y sube el resultado a MinIO).

El énfasis es la arquitectura (asincronía, aislamiento, idempotencia,
contenedores), no un motor de parsing: se usan herramientas livianas
(`pandoc`, `poppler-utils`, `pdfcpu`) ejecutadas desde el contenedor del worker.

## Arquitectura

```
┌──────────┐   multipart   ┌─────────────────────┐
│ Frontend │ ─────────────▶│         API (Go)    │
│  React   │  JWT + JSON   │  sube original      │
└──────────┘               │  crea job (pending) │
                           │  publica JobMessage │
                           └──────────┬──────────┘
                                      │ RabbitMQ (cola "jobs")
                                      ▼
                          ┌─────────────────────┐
                          │  Workers (Go) x N   │  pandoc + poppler + pdfcpu
                          │  claim atómico en DB│
                          │  convertir→segmentar│
                          │  bundle OKF→validar │
                          └──────────┬──────────┘
                                     ▼
                     MinIO: originals/ y bundles/  ·  Postgres: jobs/bundles
```

### Flujo de un trabajo

1. `POST /api/v1/jobs` sube el archivo a MinIO y publica el mensaje en la cola.
2. Un worker **reclama** el job de forma atómica (`UPDATE ... WHERE status='pending'`):
   los mensajes duplicados se descartan (idempotencia).
3. El worker descarga el original, lo convierte a Markdown y lo segmenta en
   conceptos (`fragmento-01.md`, `fragmento-02.md`, ...).
4. Genera el bundle OKF, lo valida y lo sube a MinIO.
5. El estado del job pasa a `completed` y el bundle queda disponible vía la API
   (`GET /jobs/{id}/download` para el `.zip`, o `GET /jobs/{id}/bundle/{path}`
   para archivos individuales). MinIO permanece privado: el API actúa de
   gateway para las descargas (las URLs firmadas de MinIO no se exponen al
   navegador porque su firma depende del host interno).

### Pipeline de conversión por formato

| Formato | Estrategia |
|---|---|
| **MD / TXT / HTML** | Formato base: segmentación por encabezados con el AST de goldmark. En `.txt` se promueven a encabezado las líneas que actúan como título (MAYÚSCULAS o numeradas); en `.html` se convierte a Markdown antes de segmentar. No requiere binarios externos. |
| **EPUB** | `archive/zip` + `toc.ncx`/`nav.xhtml` para el orden de capítulos + `html-to-markdown` por capítulo. |
| **DOCX** | `pandoc input.docx -t markdown` y segmentación por encabezados `#`/`##` con el AST de goldmark. |
| **PDF** | 3 niveles de fallback: ① marcadores (`pdfcpu outline`), ② heurística de tamaño de fuente (`pdftohtml -xml`), ③ rangos de N páginas (`pdftotext`). |

### Bundle OKF

```
bundles/<userId>/<jobId>/
├── index.md            # tabla de contenidos con links relativos
├── log.md              # SHA-256, tamaño, formato, recursos, fecha
├── conceptos/
│   ├── fragmento-01.md
│   └── ...
└── assets/             # recursos extraídos, referenciados como ../assets/<x>
    └── diagrama.png
```

## Alcance opcional implementado

Además del alcance mínimo, están cubiertos los siguientes puntos de la
sección 5.2 del enunciado:

- **Varios formatos de entrada**: `md`, `txt`, `html`, `pdf`, `docx`, `epub`.
- **Extracción de recursos a `assets/`** con referencia desde los conceptos.
- **Reintento idempotente** de trabajos fallidos, con vínculo al anterior.
- **Cancelación** de trabajos en curso.
- **Conformidad OKF separada de la validez de plataforma** (válido / válido con
  advertencias / inválido).
- **Métricas y observabilidad** del flujo (JSON y Prometheus).
- **Descarga por flujo**: el `.zip` se construye en streaming desde MinIO, sin
  materializar el paquete completo en memoria de la API.

## Requisitos

- Docker + Docker Compose v2
- (Opciónale) Go 1.22+ para desarrollo local

## Despliegue

```bash
cp .env.example .env     # ajustar secretos
docker compose up --build -d
```

Los puertos publicados en el host son configurables por si alguno está ocupado
(`API_PORT`, `FRONTEND_PORT`, `POSTGRES_PORT`, `MINIO_PORT`, `MINIO_CONSOLE_PORT`,
`RABBITMQ_PORT`, `RABBITMQ_UI_PORT` en `.env`). Los servicios se hablan entre sí
por la red interna de Docker, así que cambiar un puerto del host no rompe nada.

Servicios:

| Servicio | URL |
|---|---|
| Frontend | http://localhost:3000 |
| API | http://localhost:8080 (`GET /healthz`) |
| RabbitMQ UI | http://localhost:15672 (`guest`/`guest`) |
| MinIO console | http://localhost:9001 (`minioadmin`/`minioadmin`) |
| Postgres | `localhost:5432` (`okf_user`/`okf_password`/`okf_db`) |

## API

| Método | Ruta | Auth | Descripción |
|---|---|---|---|
| POST | `/api/v1/auth/register` | — | Crear usuario (email + password ≥ 8) |
| POST | `/api/v1/auth/login` | — | Obtener token JWT |
| POST | `/api/v1/jobs` | Bearer | Subir archivo (`multipart`, campo `file`): `.md`, `.txt`, `.html`, `.pdf`, `.docx`, `.epub` |
| GET | `/api/v1/jobs` | Bearer | Listar trabajos del usuario |
| GET | `/api/v1/jobs/{id}` | Bearer | Estado del trabajo (+ archivos del bundle si está completo) |
| POST | `/api/v1/jobs/{id}/cancel` | Bearer | Cancela un trabajo pendiente o en curso |
| POST | `/api/v1/jobs/{id}/retry` | Bearer | Reintenta un trabajo **fallido o cancelado** (idempotente) |
| GET | `/api/v1/jobs/{id}/download` | Bearer | Descarga el bundle completo como `.zip` |
| GET | `/api/v1/jobs/{id}/bundle/{path}` | Bearer | Archivo individual del bundle (streaming desde MinIO) |
| GET | `/api/v1/metrics` | — | Métricas agregadas del flujo (JSON) |
| GET | `/metrics` | — | Las mismas métricas en formato Prometheus |

## Desarrollo local

```bash
make api      # go run ./api/main.go
make worker   # go run ./worker/main.go
make test     # go test ./...
```

El frontend en dev:

```bash
cd frontend && npm install && npm run dev   # http://localhost:5173 (proxy a :8080)
```

## Estructura del repositorio

```
├── api/                        # API Go (REST)
│   └── internal/
│       ├── config/             # variables de entorno
│       ├── handler/            # Auth, Jobs, Bundles
│       ├── middleware/         # JWT, CORS, Logging
│       ├── repository/         # PostgreSQL (pgx)
│       ├── queue/              # Publisher RabbitMQ
│       └── storage/            # Cliente MinIO (presigned URLs)
├── worker/                     # Workers Go (consumidores)
│   └── internal/
│       ├── consumer/           # Suscriptor RabbitMQ (ack/nack, dead-letter)
│       ├── converter/          # epub.go, docx.go, pdf.go, segmenter.go
│       ├── okf/                # builder.go, validator.go
│       ├── repository/         # claim atómico + estados
│       └── storage/            # descarga originales / subida bundles
├── pkg/                        # Código compartido
│   ├── domain/                 # modelos + enums
│   └── logger/                 # slog JSON
├── frontend/                   # React + Vite + TS
├── scripts/init.sql            # esquema PostgreSQL
├── docker-compose.yml          # orquestación (postgres, minio, rabbitmq, api, worker, frontend)
└── .env.example                # plantilla de variables
```

## Validación y clasificación del resultado

La validación distingue la **validez de plataforma** (¿se puede publicar?) de la
**conformidad OKF** (¿es un buen bundle?):

| Nivel | Significado | ¿Se publica? |
|---|---|---|
| `invalid` | Falta `index.md`/`log.md`, `conceptos/` vacío o un link del índice no resuelve | **No.** El job queda `failed` y no se habilita la descarga |
| `valid_with_warnings` | Estructura correcta, pero con observaciones (concepto huérfano, sin encabezado, casi sin contenido, `log.md` sin trazabilidad) | Sí, marcado con las advertencias |
| `valid` | Estructura mínima correcta y sin observaciones | Sí |

El nivel y las advertencias se persisten en `bundles.validation` / `bundles.warnings`,
se devuelven en `GET /jobs/{id}` y el frontend los muestra al abrir los archivos.

## Reintento de trabajos fallidos

`POST /jobs/{id}/retry` crea un **nuevo** trabajo que reutiliza el original ya
almacenado en MinIO (no hay que volver a subir el archivo) y queda vinculado al
anterior mediante `retry_of`, con `attempt` incrementado.

Es **idempotente**: si el trabajo ya tiene un reintento, se devuelve ese mismo
(`200`) en lugar de crear otro. Solo aplica a trabajos en estado `failed`
(`409` en caso contrario) y respeta el aislamiento por propietario (`403`).

## Cancelación de trabajos

`POST /jobs/{id}/cancel` cancela un trabajo `pending` o `processing`. El
`UPDATE` es condicional (`WHERE status IN ('pending','processing')`), lo que lo
hace atómico frente a un worker que esté completando el mismo trabajo: gana
quien escriba primero.

Si la conversión ya estaba en curso, el worker la termina pero **comprueba la
cancelación justo antes de subir el bundle** (el punto de no retorno) y no
publica nada. `MarkCompleted`/`MarkFailed` filtran por estado para no pisar la
cancelación. Un trabajo cancelado puede reintentarse.

## Recursos (assets)

Las imágenes incrustadas en el documento se extraen a `assets/` y las
referencias de los conceptos se reescriben a `../assets/<archivo>`:

| Formato | De dónde salen |
|---|---|
| **DOCX** | `pandoc --extract-media` |
| **EPUB** | archivos de imagen del propio zip |
| **MD / HTML / TXT** | imágenes locales junto al documento |

Los recursos se deduplican por origen y los nombres se sanean; una colisión de
nombres entre directorios distintos se desambigua con un hash corto. Una
referencia rota no invalida el bundle: genera una advertencia de conformidad.

## Métricas y observabilidad

`GET /api/v1/metrics` (JSON) y `GET /metrics` (formato Prometheus) exponen el
estado del flujo: trabajos por estado y por formato, bundles por clasificación
de validación, duración media de conversión, reintentos y usuarios. Son
agregados de toda la plataforma — sin datos de ningún usuario concreto — por
lo que no requieren autenticación. El frontend los muestra en un panel plegable.

## Escalabilidad y estado

- **API sin estado**: el archivo subido viaja del request a MinIO en streaming
  (`PutStream`), sin escribirse nunca en el disco del contenedor. La API no
  guarda trabajos ni archivos en memoria ni en disco local, así que puede
  replicarse sin coordinación.
- **Workers escalables**: `docker compose up --scale worker=N`. Cada worker
  atiende hasta `prefetch` (3) trabajos **en paralelo**, acotado por un
  semáforo que coincide con el QoS de RabbitMQ; un documento largo no bloquea
  a los demás. Al apagar, los trabajos en curso se drenan antes de salir.
- **Reparto de trabajo**: lo hace la cola. El claim atómico en base de datos
  garantiza que un trabajo lo procese un único worker aunque el mensaje se
  entregue más de una vez.

## Notas operativas

- **Dead-letter**: mensajes fallidos van a `jobs.dlq` (sin reprocesar). El estado
  del job queda en `failed` con motivo en `error_message`.
- **PDF nivel 3**: usa `PDF_CHUNK_PAGES` (default 10). Garantiza que todo flujo
  produzca un bundle válido.
- **Aislamiento**: cada trabajo se procesa en un directorio temporal eliminado
  al terminar; los workers no comparten estado (solo DB y MinIO).

## Verificación rápida

Con el stack arriba (sustituye `8080` por tu `API_PORT` si lo cambiaste):

```bash
# 1. Registro -> devuelve un token JWT
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/register   -H 'Content-Type: application/json'   -d '{"email":"demo@okf.test","password":"password123"}' | jq -r .token)

# 2. Carga: responde de inmediato con el id y estado "pending" (asincronía)
JOB=$(curl -s -X POST http://localhost:8080/api/v1/jobs   -H "Authorization: Bearer $TOKEN" -F "file=@documento.md" | jq -r .id)

# 3. Estado + archivos del bundle
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/jobs/$JOB | jq

# 4. Descarga del bundle completo
curl -s -H "Authorization: Bearer $TOKEN" -o bundle.zip   http://localhost:8080/api/v1/jobs/$JOB/download
```

Pruebas unitarias (no requieren el stack):

```bash
make test     # go test ./...
```

### Documentos de prueba

`scripts/testdata/generate.sh` genera un corpus que cubre todos los formatos y
los casos exigentes de la spec (documento breve sin divisiones, documento
estructurado, PDF con y sin marcadores):

```bash
bash scripts/testdata/generate.sh     # -> scripts/testdata/out/
```

Los PDF se construyen con `scripts/testdata/make_pdf.py` (sin dependencias
externas): uno con marcadores, que ejercita el nivel 1 del conversor, y otro
sin ellos, que cae a los niveles 2/3.

### Prueba de extremo a extremo

`scripts/e2e.sh` verifica contra el sistema desplegado las condiciones de la
sección 6 del enunciado (asincronía, documento breve, documento estructurado,
aislamiento, descarga, reintento y clasificación del bundle):

```bash
bash scripts/testdata/generate.sh          # una vez
bash scripts/e2e.sh http://localhost:8080  # ajusta el puerto a tu API_PORT
```

### Condiciones verificables

| Condición | Cómo comprobarla |
|---|---|
| Asincronía | El `POST /jobs` responde `pending` de inmediato; el bundle aparece después. |
| Documento breve | Un `.md`/`.txt` sin encabezados produce `index.md`, `log.md` y **un** concepto, sin error. |
| Documento estructurado | Un documento con secciones produce un concepto por unidad, enlazados en orden desde `index.md`. |
| Bundle incompleto | `okf.Validate` exige `index.md`, `log.md`, `conceptos/` no vacío y links resolubles; si falla, el job queda `failed` y no se publica. |
| Aislamiento | Con el token de otro usuario, `GET /jobs/{id}`, `/download` y `/bundle/{path}` responden `403`; sin token, `401`. |
| Reintento idempotente | `POST /jobs/{id}/retry` sobre un job `failed` crea un job vinculado (`retry_of`, `attempt+1`); repetirlo devuelve el mismo (`200`), no crea otro. |
| Cancelación | `POST /jobs/{id}/cancel` sobre un trabajo en curso lo deja en `canceled`; el worker no publica bundle y la descarga responde `409`. |
| Assets | Un DOCX/EPUB con imágenes produce `assets/` en el bundle y los conceptos las referencian como `../assets/<archivo>`. |
| Sin duplicados | Reencolar el mismo `job_id` no crea un segundo bundle: el claim atómico lo descarta (log `job ya procesado o en progreso`). |
