package tools

// Tool categories group MCP tools by domain for client (Hermes) reasoning.
// They are metadata only: no dispatch or business logic depends on a category.
const (
	CategoryInventory   = "inventory"
	CategoryWorkflow    = "workflow" // reserved for future workflow tools
	CategoryDeployment  = "deployment"
	CategoryDiagnostics = "diagnostics"
	CategorySystem      = "system"
)
