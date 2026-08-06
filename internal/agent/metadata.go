package agent

import (
	"errors"
	"fmt"
)

// Category groups a capability by semantic domain. Categories are typed
// constants so tooling can rely on a closed vocabulary. They are metadata
// only: no dispatch, approval or business logic depends on a category.
type Category string

const (
	CategorySystem      Category = "system"
	CategoryFilesystem  Category = "filesystem"
	CategoryDocker      Category = "docker"
	CategoryGit         Category = "git"
	CategoryPM2         Category = "pm2"
	CategorySystemd     Category = "systemd"
	CategoryNetwork     Category = "network"
	CategoryDatabase    Category = "database"
	CategoryHTTP        Category = "http"
	CategoryWorkflow    Category = "workflow"
	CategoryDeployment  Category = "deployment"
	CategoryDiagnostics Category = "diagnostics"
	CategoryMonitoring  Category = "monitoring"
)

// RiskLevel describes the operational impact of executing a capability. It is
// metadata for AI reasoning and tool discovery, never an access-control
// mechanism (that is ConfirmationLevel).
type RiskLevel string

const (
	RiskReadOnly    RiskLevel = "read_only"
	RiskMutating    RiskLevel = "mutating"
	RiskDestructive RiskLevel = "destructive"
)

// EstimatedDuration is a coarse, semantic estimate of how long a capability
// usually takes. It is metadata only and is never a runtime deadline.
type EstimatedDuration string

const (
	DurationInstant EstimatedDuration = "instant"
	DurationShort   EstimatedDuration = "short"
	DurationMedium  EstimatedDuration = "medium"
	DurationLong    EstimatedDuration = "long"
)

// ToolMetadata is the semantic metadata of a registered capability. It exists
// to support AI reasoning, tool discovery, filtering and documentation; it is
// never consulted for execution, approval or dispatch decisions.
//
// The Registry is the single source of truth for Name, Description and
// RequiresConfirmation: tools derive them from their own methods and the
// registry canonicalizes them on registration. SinceVersion falls back to
// Version() when a tool does not provide one.
type ToolMetadata struct {
	Name                 string
	Description          string
	Category             Category
	Domain               string
	Tags                 []string
	Risk                 RiskLevel
	RequiresConfirmation bool
	EstimatedDuration    EstimatedDuration
	SinceVersion         string
}

// Validate rejects metadata that cannot be reasoned about: a missing name,
// description or category, an unknown risk, or duplicate tags. The registry
// runs it on every registration so invalid metadata fails at startup.
func (m ToolMetadata) Validate() error {
	switch {
	case m.Name == "":
		return errors.New("metadata: name is required")
	case m.Description == "":
		return errors.New("metadata: description is required")
	case m.Category == "":
		return errors.New("metadata: category is required")
	case !ValidRisk(m.Risk):
		return fmt.Errorf("metadata: invalid risk %q", m.Risk)
	case hasDuplicateTags(m.Tags):
		return fmt.Errorf("metadata: duplicate tags in %v", m.Tags)
	case m.EstimatedDuration == "":
		return errors.New("metadata: estimated_duration is required")
	}
	return nil
}

// ValidRisk reports whether r is one of the known risk levels.
func ValidRisk(r RiskLevel) bool {
	switch r {
	case RiskReadOnly, RiskMutating, RiskDestructive:
		return true
	default:
		return false
	}
}

func hasDuplicateTags(tags []string) bool {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag == "" {
			return true
		}
		if _, ok := seen[tag]; ok {
			return true
		}
		seen[tag] = struct{}{}
	}
	return false
}
