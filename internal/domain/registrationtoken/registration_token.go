package registrationtoken

import (
	"time"

	"github.com/google/uuid"
)

type RegistrationToken struct {
	ID          uuid.UUID
	TokenHash   string
	Environment *string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}
