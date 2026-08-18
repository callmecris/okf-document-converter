package repository

import (
	"context"
	"fmt"

	"okf/pkg/domain"
)

type CreateJobParams struct {
	ID           string
	UserID       string
	OriginalName string
	Format       domain.DocFormat
	ObjectKey    string
}

func (p *Postgres) CreateJob(ctx context.Context, params CreateJobParams) (domain.Job, error) {
	j := domain.Job{}
	err := p.pool.QueryRow(ctx, `
		INSERT INTO jobs (id, user_id, original_name, format, object_key, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, original_name, format, object_key, status,
		          COALESCE(error_message, ''), created_at, updated_at`,
		params.ID, params.UserID, params.OriginalName, params.Format, params.ObjectKey, domain.JobStatusPending,
	).Scan(&j.ID, &j.UserID, &j.OriginalName, &j.Format, &j.ObjectKey, &j.Status, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return domain.Job{}, fmt.Errorf("insert job: %w", err)
	}
	return j, nil
}

func (p *Postgres) GetJob(ctx context.Context, id string) (domain.Job, error) {
	j := domain.Job{}
	err := p.pool.QueryRow(ctx, `
		SELECT id, user_id, original_name, format, object_key, status,
		       COALESCE(error_message, ''), created_at, updated_at
		FROM jobs WHERE id = $1`, id,
	).Scan(&j.ID, &j.UserID, &j.OriginalName, &j.Format, &j.ObjectKey, &j.Status, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return domain.Job{}, fmt.Errorf("get job: %w", err)
	}
	return j, nil
}

func (p *Postgres) ListJobs(ctx context.Context, userID string) ([]domain.Job, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, user_id, original_name, format, object_key, status,
		       COALESCE(error_message, ''), created_at, updated_at
		FROM jobs WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := []domain.Job{}
	for rows.Next() {
		var j domain.Job
		if err := rows.Scan(&j.ID, &j.UserID, &j.OriginalName, &j.Format, &j.ObjectKey, &j.Status, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt); err != nil {
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