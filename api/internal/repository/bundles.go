package repository

import (
	"context"
	"fmt"

	"okf/pkg/domain"
)

func (p *Postgres) CreateBundle(ctx context.Context, id, jobID, path string) (domain.Bundle, error) {
	b := domain.Bundle{}
	err := p.pool.QueryRow(ctx, `
		INSERT INTO bundles (id, job_id, path)
		VALUES ($1, $2, $3)
		RETURNING id, job_id, path, created_at`, id, jobID, path,
	).Scan(&b.ID, &b.JobID, &b.Path, &b.CreatedAt)
	if err != nil {
		return domain.Bundle{}, fmt.Errorf("insert bundle: %w", err)
	}
	return b, nil
}

func (p *Postgres) GetBundleByJob(ctx context.Context, jobID string) (domain.Bundle, error) {
	b := domain.Bundle{}
	err := p.pool.QueryRow(ctx, `
		SELECT id, job_id, path, created_at FROM bundles WHERE job_id = $1`, jobID,
	).Scan(&b.ID, &b.JobID, &b.Path, &b.CreatedAt)
	if err != nil {
		return domain.Bundle{}, fmt.Errorf("get bundle: %w", err)
	}
	return b, nil
}