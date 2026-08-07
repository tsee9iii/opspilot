package alert

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ListRequest filters the alerts to return. Zero values are wildcards.
type ListRequest struct {
	Status   string
	Severity string
	AgentID  string
	ServerID string
	// Limit caps the result set. Zero applies the default cap.
	Limit int
}

// DefaultListLimit caps unfiltered alert listings to a bounded set.
const DefaultListLimit = 100

// ListUseCase reads alerts for operators and the MCP.
type ListUseCase struct {
	repo ReadRepository
}

func NewListUseCase(repo ReadRepository) *ListUseCase {
	return &ListUseCase{repo: repo}
}

func (uc *ListUseCase) List(ctx context.Context, req ListRequest) ([]Alert, error) {
	if req.AgentID != "" {
		if _, err := uuid.Parse(req.AgentID); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidAgentID, req.AgentID)
		}
	}
	if req.ServerID != "" {
		if _, err := uuid.Parse(req.ServerID); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidServerID, req.ServerID)
		}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultListLimit
	}
	return uc.repo.List(ctx, req.Status, req.Severity, req.AgentID, req.ServerID, limit)
}

// GetUseCase reads a single alert by id.
type GetUseCase struct {
	repo ReadRepository
}

func NewGetUseCase(repo ReadRepository) *GetUseCase {
	return &GetUseCase{repo: repo}
}

func (uc *GetUseCase) Get(ctx context.Context, id string) (Alert, error) {
	if id == "" {
		return Alert{}, ErrInvalidAlertID
	}
	if _, err := uuid.Parse(id); err != nil {
		return Alert{}, ErrInvalidAlertID
	}
	return uc.repo.GetByID(ctx, id)
}

// AcknowledgeUseCase acknowledges an open alert on behalf of an operator. The
// acknowledging actor is recorded and immutable; acknowledged alerts remain
// visible and unresolved until a recovery resolves them.
type AcknowledgeUseCase struct {
	repo ReadRepository
}

func NewAcknowledgeUseCase(repo ReadRepository) *AcknowledgeUseCase {
	return &AcknowledgeUseCase{repo: repo}
}

func (uc *AcknowledgeUseCase) Acknowledge(ctx context.Context, id, acknowledgedBy string) (Alert, error) {
	if acknowledgedBy == "" {
		return Alert{}, ErrAcknowledgedByRequired
	}
	if _, err := uuid.Parse(id); err != nil {
		return Alert{}, ErrInvalidAlertID
	}
	alertRow, err := uc.repo.Acknowledge(ctx, id, acknowledgedBy)
	if err != nil {
		return Alert{}, err
	}
	return alertRow, nil
}
