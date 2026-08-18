-- Esquema inicial de PostgreSQL para el proyecto OKF
-- Se ejecuta automáticamente en el primer arranque del contenedor postgres.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS jobs (
    id            UUID PRIMARY KEY,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    original_name TEXT NOT NULL,
    format        TEXT NOT NULL CHECK (format IN ('pdf', 'docx', 'epub')),
    object_key    TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    error_message TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_jobs_user_created ON jobs (user_id, created_at DESC);

-- El UPDATE del claim en el worker filtra por (status = 'pending'):
-- índice parcial para acelerar la búsqueda de trabajos reclamables.
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS bundles (
    id         UUID PRIMARY KEY,
    job_id     UUID NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);