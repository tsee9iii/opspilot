package token

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/tsee9iii/opspilot/internal/domain/registrationtoken"
)

func runList(ctx context.Context, deps Deps, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(deps.ErrOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("list: unexpected argument %q", fs.Arg(0))
	}

	tokens, err := deps.Repo.List(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(deps.Out, "%-38s %-14s %-24s %-24s %-8s %s\n",
		"ID", "Environment", "Created At", "Expires At", "Revoked", "Consumed")
	for _, t := range tokens {
		writeListRow(deps.Out, t)
	}
	return nil
}

func writeListRow(w io.Writer, t *registrationtoken.RegistrationToken) {
	environment := ""
	if t.Environment != nil {
		environment = *t.Environment
	}
	revoked := "no"
	if t.RevokedAt != nil {
		revoked = "yes"
	}
	fmt.Fprintf(w, "%-38s %-14s %-24s %-24s %-8s %s\n",
		t.ID,
		environment,
		t.CreatedAt.Format(time.RFC3339),
		t.ExpiresAt.Format(time.RFC3339),
		revoked,
		// Consumed tokens are deleted at consumption time, so any token that
		// still exists has not been consumed.
		"no",
	)
}
