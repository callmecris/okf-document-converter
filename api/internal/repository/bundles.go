package repository

import (
	"context"
	"fmt"

	"okf/pkg/domain"
)

func (p *Postgres) GetBundleByJob(ctx context.Context, jobID string) (domain.Bundle, error) {
	b := domain.Bundle{}
	err := p.pool.QueryRow(ctx, `
		SELECT id, job_id, path, validation, warnings, created_at
		FROM bundles WHERE job_id = $1`, jobID,
	).Scan(&b.ID, &b.JobID, &b.Path, &b.Validation, &b.Warnings, &b.CreatedAt)
	if err != nil {
		return domain.Bundle{}, fmt.Errorf("get bundle: %w", err)
	}
	return b, nil
}