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
   conceptos (`capitulo-01.md`, `capitulo-02.md`, ...).
4. Genera el bundle OKF, lo valida y lo sube a MinIO.
5. El estado del job pasa a `completed` y el bundle queda disponible vía la API
   (`GET /jobs/{id}/download` para el `.zip`, o `GET /jobs/{id}/bundle/{path}`
   para archivos individuales). MinIO permanece privado: el API actúa de
   gateway para las descargas (las URLs firmadas de MinIO no se exponen al
   navegador porque su firma depende del host interno).

### Pipeline de conversión por formato

| Formato | Estrategia |
|---|---|
| **EPUB** | `archive/zip` + `toc.ncx`/`nav.xhtml` para el orden de capítulos + `html-to-markdown` por capítulo. |
| **DOCX** | `pandoc input.docx -t markdown` y segmentación por encabezados `#`/`##` con el AST de goldmark. |
| **PDF** | 3 niveles de fallback: ① marcadores (`pdfcpu outline`), ② heurística de tamaño de fuente (`pdftohtml -xml`), ③ rangos de N páginas (`pdftotext`). |

### Bundle OKF

```
bundles/<userId>/<jobId>/
├── index.md            # tabla de contenidos con links relativos
├── log.md              # SHA-256, tamaño, formato, fecha de conversión
└── conceptos/
    ├── capitulo-01.md
    └── ...
```

## Requisitos

- Docker + Docker Compose v2
- (Opciónale) Go 1.22+ para desarrollo local

## Despliegue

```bash
cp .env.example .env     # ajustar secretos
docker compose up --build -d
```

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
| POST | `/api/v1/jobs` | Bearer | Subir archivo (`multipart`, campo `file`) |
| GET | `/api/v1/jobs` | Bearer | Listar trabajos del usuario |
| GET | `/api/v1/jobs/{id}` | Bearer | Estado del trabajo (+ archivos del bundle si está completo) |
| GET | `/api/v1/jobs/{id}/download` | Bearer | Descarga el bundle completo como `.zip` |
| GET | `/api/v1/jobs/{id}/bundle/{path}` | Bearer | Archivo individual del bundle (streaming desde MinIO) |

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

## Notas operativas

- **Dead-letter**: mensajes fallidos van a `jobs.dlq` (sin reprocesar). El estado
  del job queda en `failed` con motivo en `error_message`.
- **PDF nivel 3**: usa `PDF_CHUNK_PAGES` (default 10). Garantiza que todo flujo
  produzca un bundle válido.
- **Aislamiento**: cada trabajo se procesa en un directorio temporal eliminado
  al terminar; los workers no comparten estado (solo DB y MinIO).