package http

// ApproveCommandRequest is the operator approval of a pending command. The
// authenticated operator actor is not read from the body; it is taken from the
// X-Operator-Actor header, so a client cannot forge the approver identity.
type ApproveCommandRequest struct {
	CommandID string `json:"command_id"`
	// ApprovalNote is an optional human note recorded with the approval. It is
	// immutable once written.
	ApprovalNote string `json:"approval_note,omitempty"`
}
