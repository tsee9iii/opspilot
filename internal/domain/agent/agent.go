package agent

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID            uuid.UUID
	ServerID      uuid.UUID
	Secret        string
	Version       string
	Status        string
	LastHeartbeat *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
