package repository

import (
	"context"
	"fmt"

	"okf/pkg/domain"
)

// Metrics resume el estado del flujo de trabajos de la plataforma.
type Metrics struct {
	// JobsByStatus cuenta los trabajos por estado (pending, processing, ...).
	JobsByStatus map[domain.JobStatus]int `json:"jobs_by_status"`
	// JobsByFormat cuenta los trabajos por formato de entrada.
	JobsByFormat map[domain.DocFormat]int `json:"jobs_by_format"`
	// BundlesByValidation cuenta los bundles publicados por clasificación.
	BundlesByValidation map[domain.ValidationLevel]int `json:"bundles_by_validation"`
	// Totales agregados.
	TotalJobs    int `json:"total_jobs"`
	TotalBundles int `json:"total_bundles"`
	TotalUsers   int `json:"total_users"`
	Retries      int `json:"retries"`
	// AvgDurationSeconds es la duración media de las conversiones terminadas.
	AvgDurationSeconds float64 `json:"avg_duration_seconds"`
}

// GetMetrics agrega el estado del sistema en una sola pasada por tabla.
func (p *Postgres) GetMetrics(ctx context.Context) (Metrics, error) {
	m := Metrics{
		JobsByStatus:        map[domain.JobStatus]int{},
		JobsByFormat:        map[domain.DocFormat]int{},
		BundlesByValidation: map[domain.ValidationLevel]int{},
	}

	rows, err := p.pool.Query(ctx, `SELECT status, count(*) FROM jobs GROUP BY status`)
	if err != nil {
		return m, fmt.Errorf("metrics by status: %w", err)
	}
	for rows.Next() {
		var status domain.JobStatus
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return m, err
		}
		m.JobsByStatus[status] = n
		m.TotalJobs += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return m, err
	}

	rows, err = p.pool.Query(ctx, `SELECT format, count(*) FROM jobs GROUP BY format`)
	if err != nil {
		return m, fmt.Errorf("metrics by format: %w", err)
	}
	for rows.Next() {
		var format domain.DocFormat
		var n int
		if err := rows.Scan(&format, &n); err != nil {
			rows.Close()
			return m, err
		}
		m.JobsByFormat[format] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return m, err
	}

	rows, err = p.pool.Query(ctx, `SELECT validation, count(*) FROM bundles GROUP BY validation`)
	if err != nil {
		return m, fmt.Errorf("metrics by validation: %w", err)
	}
	for rows.Next() {
		var level domain.ValidationLevel
		var n int
		if err := rows.Scan(&level, &n); err != nil {
			rows.Close()
			return m, err
		}
		m.BundlesByValidation[level] = n
		m.TotalBundles += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return m, err
	}

	// Duración media de las conversiones ya terminadas (created -> updated).
	if err := p.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (updated_at - created_at))), 0)
		FROM jobs WHERE status IN ($1, $2)`,
		domain.JobStatusCompleted, domain.JobStatusFailed,
	).Scan(&m.AvgDurationSeconds); err != nil {
		return m, fmt.Errorf("metrics duration: %w", err)
	}

	if err := p.pool.QueryRow(ctx,
		`SELECT count(*) FROM jobs WHERE retry_of IS NOT NULL`).Scan(&m.Retries); err != nil {
		return m, fmt.Errorf("metrics retries: %w", err)
	}
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&m.TotalUsers); err != nil {
		return m, fmt.Errorf("metrics users: %w", err)
	}
	return m, nil
}
