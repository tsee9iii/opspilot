package command

import (
	"time"

	"github.com/google/uuid"
)

type Command struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	ToolName  string
	Payload   []byte
	Status    string
	Result    *[]byte
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
