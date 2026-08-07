package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/gen/postgresql"
	appagent "github.com/tsee9iii/opspilot/internal/application/agent"
	"github.com/tsee9iii/opspilot/internal/domain/registrationtoken"
)

type RegistrationTokenRepository struct {
	q *postgresql.Queries
}

func NewRegistrationTokenRepository(pool *pgxpool.Pool) *RegistrationTokenRepository {
	return &RegistrationTokenRepository{q: postgresql.New(pool)}
}

func (r *RegistrationTokenRepository) Create(ctx context.Context, token registrationtoken.RegistrationToken) error {
	_, err := r.q.CreateRegistrationToken(ctx, postgresql.CreateRegistrationTokenParams{
		TokenHash:   token.TokenHash,
		Environment: pgtypeText(token.Environment),
		ExpiresAt:   token.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create registration token: %w", err)
	}
	return nil
}

func (r *RegistrationTokenRepository) FindByHash(ctx context.Context, tokenHash string) (*registrationtoken.RegistrationToken, error) {
	row, err := r.q.GetRegistrationTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appagent.ErrTokenNotFound
		}
		return nil, fmt.Errorf("postgres: get registration token: %w", err)
	}
	return mapRegistrationToken(row), nil
}

func (r *RegistrationTokenRepository) Consume(ctx context.Context, tokenHash string) (bool, error) {
	_, err := r.q.DeleteRegistrationTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("postgres: delete registration token: %w", err)
	}
	return true, nil
}

func (r *RegistrationTokenRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	if err := r.q.RevokeRegistrationToken(ctx, id); err != nil {
		return fmt.Errorf("postgres: revoke registration token: %w", err)
	}
	return nil
}

func (r *RegistrationTokenRepository) List(ctx context.Context) ([]*registrationtoken.RegistrationToken, error) {
	rows, err := r.q.ListRegistrationTokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list registration tokens: %w", err)
	}
	tokens := make([]*registrationtoken.RegistrationToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, mapRegistrationToken(row))
	}
	return tokens, nil
}

func mapRegistrationToken(row postgresql.RegistrationTokens) *registrationtoken.RegistrationToken {
	return &registrationtoken.RegistrationToken{
		ID:          row.ID,
		TokenHash:   row.TokenHash,
		Environment: pgtypeTextPtr(row.Environment),
		ExpiresAt:   row.ExpiresAt,
		RevokedAt:   pgtypeTimePtr(row.RevokedAt),
		CreatedAt:   row.CreatedAt,
	}
}

func pgtypeText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func pgtypeTextPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func pgtypeTimePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func pgtypeUUID(v uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: v, Valid: true}
}

func pgtypeUUIDValue(v pgtype.UUID) uuid.UUID {
	return uuid.UUID(v.Bytes)
}
