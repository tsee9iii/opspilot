package token

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/google/uuid"
)

func runRevoke(ctx context.Context, deps Deps, args []string) error {
	fs := flag.NewFlagSet("revoke", flag.ContinueOnError)
	fs.SetOutput(deps.ErrOut)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("revoke requires exactly one token id")
	}

	id, err := uuid.Parse(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("revoke: invalid token id %q: %w", fs.Arg(0), err)
	}

	if err := deps.Repo.Revoke(ctx, id); err != nil {
		return err
	}

	fmt.Fprintf(deps.Out, "revoked %s\n", id)
	return nil
}
