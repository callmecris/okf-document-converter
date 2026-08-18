// Package repository contiene el acceso a PostgreSQL del worker.
// Incluye el claim atómico de trabajos (base de la idempotencia).
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"okf/pkg/domain"
)

// Postgres envuelve el pool de conexiones del worker.
type Postgres struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *Postgres) Close() {
	p.pool.Close()
}

// ClaimJob marca un job como processing de forma atómica.
// Devuelve false si el job no estaba en estado pending (duplicado o fuera de orden),
// permitiendo ACK del mensaje sin reprocesarlo.
func (p *Postgres) ClaimJob(ctx context.Context, jobID string) (bool, error) {
	tag, err := p.pool.Exec(ctx, `
		UPDATE jobs SET status = $1, updated_at = now()
		WHERE id = $2 AND status = $3`,
		domain.JobStatusProcessing, jobID, domain.JobStatusPending)
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// GetJobName devuelve los datos mínimos del job (para metadatos del bundle).
func (p *Postgres) GetJobName(ctx context.Context, jobID string) (domain.Job, error) {
	j := domain.Job{}
	err := p.pool.QueryRow(ctx, `
		SELECT id, user_id, original_name, format, object_key, status,
		       COALESCE(error_message, ''), created_at, updated_at
		FROM jobs WHERE id = $1`, jobID,
	).Scan(&j.ID, &j.UserID, &j.OriginalName, &j.Format, &j.ObjectKey, &j.Status, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return domain.Job{}, fmt.Errorf("get job: %w", err)
	}
	return j, nil
}

func (p *Postgres) MarkCompleted(ctx context.Context, jobID string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE jobs SET status = $1, updated_at = now() WHERE id = $2`,
		domain.JobStatusCompleted, jobID)
	if err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	return nil
}

func (p *Postgres) MarkFailed(ctx context.Context, jobID, errMsg string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE jobs SET status = $1, error_message = $2, updated_at = now() WHERE id = $3`,
		domain.JobStatusFailed, errMsg, jobID)
	if err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}
	return nil
}

func (p *Postgres) CreateBundle(ctx context.Context, id, jobID, path string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO bundles (id, job_id, path) VALUES ($1, $2, $3)`,
		id, jobID, path)
	if err != nil {
		return fmt.Errorf("insert bundle: %w", err)
	}
	return nil
}