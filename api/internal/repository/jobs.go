package repository

import (
	"context"
	"fmt"

	"okf/pkg/domain"
)

// jobColumns es la proyección compartida por todas las consultas de jobs.
const jobColumns = `id, user_id, original_name, format, object_key, status,
	       COALESCE(error_message, ''), retry_of, attempt, created_at, updated_at`

func scanJob(row interface {
	Scan(dest ...any) error
}) (domain.Job, error) {
	j := domain.Job{}
	err := row.Scan(&j.ID, &j.UserID, &j.OriginalName, &j.Format, &j.ObjectKey,
		&j.Status, &j.ErrorMessage, &j.RetryOf, &j.Attempt, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

type CreateJobParams struct {
	ID           string
	UserID       string
	OriginalName string
	Format       domain.DocFormat
	ObjectKey    string
	// RetryOf y Attempt solo se usan al reintentar un trabajo fallido.
	RetryOf *string
	Attempt int
}

func (p *Postgres) CreateJob(ctx context.Context, params CreateJobParams) (domain.Job, error) {
	attempt := params.Attempt
	if attempt < 1 {
		attempt = 1
	}
	j, err := scanJob(p.pool.QueryRow(ctx, `
		INSERT INTO jobs (id, user_id, original_name, format, object_key, status, retry_of, attempt)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+jobColumns,
		params.ID, params.UserID, params.OriginalName, params.Format,
		params.ObjectKey, domain.JobStatusPending, params.RetryOf, attempt,
	))
	if err != nil {
		return domain.Job{}, fmt.Errorf("insert job: %w", err)
	}
	return j, nil
}

func (p *Postgres) GetJob(ctx context.Context, id string) (domain.Job, error) {
	j, err := scanJob(p.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1`, id))
	if err != nil {
		return domain.Job{}, fmt.Errorf("get job: %w", err)
	}
	return j, nil
}

func (p *Postgres) ListJobs(ctx context.Context, userID string) ([]domain.Job, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT `+jobColumns+` FROM jobs WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := []domain.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus permite al API marcar un job fallido si la publicación en cola falla.
func (p *Postgres) UpdateJobStatus(ctx context.Context, jobID string, status domain.JobStatus) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE jobs SET status = $1, updated_at = now() WHERE id = $2`, status, jobID)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

// CancelJob marca como cancelado un trabajo que aún no ha terminado.
// El UPDATE condicional lo hace atómico frente a un worker que esté
// completando el mismo trabajo en ese instante: gana quien escriba primero.
// Devuelve false si el trabajo ya no era cancelable.
func (p *Postgres) CancelJob(ctx context.Context, jobID string) (bool, error) {
	tag, err := p.pool.Exec(ctx, `
		UPDATE jobs SET status = $1, updated_at = now()
		WHERE id = $2 AND status IN ($3, $4)`,
		domain.JobStatusCanceled, jobID, domain.JobStatusPending, domain.JobStatusProcessing)
	if err != nil {
		return false, fmt.Errorf("cancel job: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// FindRetryOf devuelve el reintento ya existente de un trabajo, si lo hay.
// Hace idempotente el endpoint de reintento: reintentar dos veces el mismo
// trabajo no crea dos jobs nuevos.
func (p *Postgres) FindRetryOf(ctx context.Context, jobID string) (domain.Job, bool, error) {
	j, err := scanJob(p.pool.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM jobs WHERE retry_of = $1 ORDER BY created_at DESC LIMIT 1`, jobID))
	if err != nil {
		return domain.Job{}, false, nil
	}
	return j, true, nil
}
