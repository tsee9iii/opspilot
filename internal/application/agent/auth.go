package agent

import (
	"context"
	"errors"

	"github.com/tsee9iii/opspilot/internal/domain/registrationtoken"
)

var (
	ErrTokenNotFound = errors.New("registration token not found")
	ErrTokenExpired  = errors.New("registration token expired")
	ErrTokenRevoked  = errors.New("registration token revoked")
	ErrTokenUsed     = errors.New("registration token already used")
)

// TokenRepository defines the persistence contract for registration tokens.
type TokenRepository interface {
	FindByHash(ctx context.Context, tokenHash string) (*registrationtoken.RegistrationToken, error)
	Consume(ctx context.Context, tokenHash string) (bool, error)
}

// TokenHasher computes the HMAC of a registration token.
type TokenHasher interface {
	Hash(token string) string
}

// SecretHasher hashes and verifies agent secrets using Argon2id.
type SecretHasher interface {
	Hash(ctx context.Context, secret string) (string, error)
	Verify(ctx context.Context, encoded string, secret string) (bool, error)
}
