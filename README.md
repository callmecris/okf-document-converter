# OKF — Conversión de Documentos (PDF, DOCX, EPUB) a Bundles Markdown

## Datos de entrega

| | |
|---|---|
| **Proyecto** | OKF — Conversión de documentos a bundles Markdown |
| **Integrante 1** | *Wyo Hann Chu Mendez* |
| **Integrante 2** | *Cristian Felipe Ochoa Osorio* |
| **Curso / Asignatura** | *Desarrollo de Soluciones Cloud* |
| **Fecha de entrega** | *2026-08-23* |

> Repositorio entregado con el sistema desplegable íntegro vía Docker Compose.


## Descripción general

Arquitectura cloud asíncrona: la API en Go encola trabajos en RabbitMQ y los
workers los procesan de forma aislada (cada worker convierte un documento en un
directorio temporal y sube el resultado a MinIO). La conversión usa herramientas
livianas (`pandoc`, `poppler-utils`, `pdfcpu`); el énfasis está en la
arquitectura: asincronía, idempotencia, aislamiento multiusuario y contenedores.

Cada documento produce un **bundle OKF**:

```
bundles/<userId>/<jobId>/
├── index.md            # tabla de contenidos con links relativos
├── log.md              # SHA-256 del original, formato, recursos, fecha
├── conceptos/
│   ├── fragmento-01.md # un concepto por unidad detectada (encabezados)
│   └── ...
└── assets/             # imágenes extraídas, referenciadas como ../assets/<x>
```

El bundle se valida antes de publicarse (válido / válido con advertencias /
inválido — un bundle inválido nunca queda disponible para descarga) y se
descarga como `.zip` o archivo por archivo a través de la API, que actúa de
gateway privado de MinIO.

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

Flujo resumido:

1. `POST /api/v1/jobs` sube el archivo a MinIO, crea el job en Postgres y
   publica el mensaje en la cola; responde de inmediato con el `id`.
2. Un worker reclama el job de forma atómica (`UPDATE ... WHERE status='pending'`),
   lo que hace idempotente el sistema ante mensajes duplicados.
3. El worker descarga el original, lo convierte, segmenta en conceptos,
   construye y valida el bundle, y lo sube a MinIO solo si es publicable.
4. El frontend consulta el estado (`GET /jobs/{id}` cada pocos segundos) y
   descarga el resultado cuando el job pasa a `completed`.

La API no guarda estado (JWT sin sesiones, nada en memoria ni disco local), así
que puede replicarse libremente; los workers escalan con
`docker compose up --scale worker=N`.

## Despliegue

Requisitos: Docker + Docker Compose v2.

```bash
cp .env.example .env     # ajustar secretos
docker compose up --build -d
```

Los puertos del host son configurables en `.env` (`API_PORT`, `FRONTEND_PORT`,
`POSTGRES_PORT`, `MINIO_PORT`, `MINIO_CONSOLE_PORT`, `RABBITMQ_PORT`,
`RABBITMQ_UI_PORT`); los servicios se comunican por la red interna de Docker.

| Servicio | URL |
|---|---|
| Frontend | http://localhost:3000 |
| API | http://localhost:8080 (`GET /healthz`) |
| RabbitMQ UI | http://localhost:15672 (`guest`/`guest`) |
| MinIO console | http://localhost:9001 (`minioadmin`/`minioadmin`) |
| Postgres | `localhost:5432` (`okf_user`/`okf_password`/`okf_db`) |

Verificación rápida una vez levantado el stack:

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@okf.test","password":"password123"}' | jq -r .token)

JOB=$(curl -s -X POST http://localhost:8080/api/v1/jobs \
  -H "Authorization: Bearer $TOKEN" -F "file=@documento.pdf" | jq -r .id)

curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/jobs/$JOB | jq

curl -s -H "Authorization: Bearer $TOKEN" -o bundle.zip \
  http://localhost:8080/api/v1/jobs/$JOB/download
```

Pruebas unitarias (sin necesidad del stack): `make test`.

Documentos de prueba: `bash scripts/testdata/generate.sh` genera un corpus que
cubre todos los formatos en `scripts/testdata/out/`.

## Estructura del repositorio

```
├── api/                        # API Go (REST)
│   └── internal/
│       ├── config/             # variables de entorno
│       ├── handler/            # Auth, Jobs, Bundles, Metrics
│       ├── middleware/         # JWT, CORS, Logging
│       ├── repository/         # PostgreSQL (pgx)
│       ├── queue/              # Publisher RabbitMQ
│       └── storage/            # Cliente MinIO
├── worker/                     # Workers Go (consumidores)
│   └── internal/
│       ├── consumer/           # Suscriptor RabbitMQ (ack/nack, dead-letter)
│       ├── converter/          # epub.go, docx.go, pdf.go, segmenter.go, assets.go
│       ├── okf/                # builder.go, validator.go
│       ├── repository/         # claim atómico + estados del job
│       └── storage/            # descarga originales / subida bundles
├── pkg/                        # Código compartido
│   ├── domain/                 # modelos + enums (JobMessage, estados…)
│   └── logger/                 # slog JSON
├── frontend/                   # React + Vite + TS
├── docs/video-guion.md         # guion del video de sustentación
├── scripts/init.sql            # esquema PostgreSQL
├── scripts/testdata/           # generador de corpus de prueba + e2e.sh
├── docker-compose.yml          # postgres · minio · rabbitmq · api · worker · frontend
└── .env.example                # plantilla de variables
```
