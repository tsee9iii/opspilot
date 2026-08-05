// Package token implements the `opspilot-central token` subcommands. It is the
// official operator interface for managing registration tokens and reuses the
// existing domain, repository, and hashing implementations.
package token

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/internal/domain/registrationtoken"
	"github.com/tsee9iii/opspilot/internal/infrastructure/postgres"
	"github.com/tsee9iii/opspilot/internal/infrastructure/security"
	"github.com/tsee9iii/opspilot/pkg/config"
)

// Repository is the subset of the registration-token repository the CLI needs.
type Repository interface {
	Create(ctx context.Context, token registrationtoken.RegistrationToken) error
	List(ctx context.Context) ([]*registrationtoken.RegistrationToken, error)
	Revoke(ctx context.Context, id uuid.UUID) error
}

// Hasher computes the HMAC of a registration token.
type Hasher interface {
	Hash(token string) string
}

// Deps carries the dependencies the subcommands run against.
type Deps struct {
	Repo   Repository
	Hasher Hasher
	Out    io.Writer
	ErrOut io.Writer
}

// defaultExpiry is used when --expires is omitted.
const defaultExpiry = 30 * 24 * time.Hour

// Run executes the token CLI against real infrastructure and returns a process
// exit code. It exits with 1 on a clear, operator-facing error and 2 on usage
// errors.
func Run(ctx context.Context, args []string) int {
	pool, cfg, err := open(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "central token: %v\n", err)
		return 1
	}
	defer pool.Close()

	deps := Deps{
		Repo:   postgres.NewRegistrationTokenRepository(pool),
		Hasher: security.NewHMACHasher(cfg.Auth.ServerSecret),
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
	if err := dispatch(ctx, deps, args); err != nil {
		fmt.Fprintf(os.Stderr, "central token: %v\n", err)
		return 1
	}
	return 0
}

// open loads the shared configuration and connects to PostgreSQL, reusing the
// exact initialization Central uses. It returns a clear error when the database
// cannot be reached.
func open(ctx context.Context) (*pgxpool.Pool, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	pool, err := postgres.New(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("init postgres: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("connect database: %w", err)
	}

	return pool, cfg, nil
}

// dispatch routes to the requested subcommand.
func dispatch(ctx context.Context, deps Deps, args []string) error {
	if len(args) == 0 {
		usage(deps.ErrOut)
		return errors.New("missing subcommand")
	}
	switch args[0] {
	case "create":
		return runCreate(ctx, deps, args[1:])
	case "list":
		return runList(ctx, deps, args[1:])
	case "revoke":
		return runRevoke(ctx, deps, args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: opspilot-central token <create|list|revoke>")
	fmt.Fprintln(w, "  create               create a registration token")
	fmt.Fprintln(w, "  list                 list registration tokens")
	fmt.Fprintln(w, "  revoke <token-id>    revoke a registration token")
}
