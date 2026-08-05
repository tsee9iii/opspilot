package server

import (
	"time"

	"github.com/google/uuid"
)

type Server struct {
	ID          uuid.UUID
	Name        string
	Hostname    string
	Environment string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
