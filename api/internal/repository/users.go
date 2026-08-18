package repository

import (
	"context"
	"fmt"

	"okf/pkg/domain"
)

type CreateUserParams struct {
	ID           string
	Email        string
	PasswordHash string
}

func (p *Postgres) CreateUser(ctx context.Context, params CreateUserParams) (domain.User, error) {
	u := domain.User{}
	err := p.pool.QueryRow(ctx, `
		INSERT INTO users (id, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, created_at`,
		params.ID, params.Email, params.PasswordHash,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (p *Postgres) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	return p.getUser(ctx, `SELECT id, email, password_hash, created_at FROM users WHERE email = $1`, email)
}

func (p *Postgres) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	return p.getUser(ctx, `SELECT id, email, password_hash, created_at FROM users WHERE id = $1`, id)
}

func (p *Postgres) getUser(ctx context.Context, query, arg string) (domain.User, error) {
	u := domain.User{}
	err := p.pool.QueryRow(ctx, query, arg).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}