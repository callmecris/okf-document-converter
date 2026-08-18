// Package repository contiene el acceso a PostgreSQL de la API (pgx v5).
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres envuelve el pool de conexiones compartido por los repositorios.
type Postgres struct {
	pool *pgxpool.Pool
}

// New conecta a PostgreSQL usando el DSN provisto.
func New(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Ping verifica que la base de datos responda.
func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Close cierra el pool.
func (p *Postgres) Close() {
	p.pool.Close()
}

// Exec ejecuta una sentencia sin resultados.
func (p *Postgres) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := p.pool.Exec(ctx, sql, args...)
	return err
}