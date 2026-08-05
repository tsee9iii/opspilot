package token

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tsee9iii/opspilot/internal/domain/registrationtoken"
)

const tokenPrefix = "ops_rt_"

// tokenBytes is the number of bytes of cryptographically secure entropy in each
// token. 32 bytes satisfy the 256-bit minimum.
const tokenBytes = 32

func runCreate(ctx context.Context, deps Deps, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(deps.ErrOut)
	environment := fs.String("environment", "production", "environment the token is valid for")
	expires := fs.String("expires", "", "token lifetime, e.g. 24h, 7d, 30d (default 30d)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("create: unexpected argument %q", fs.Arg(0))
	}

	lifetime, err := parseLifetime(*expires)
	if err != nil {
		return err
	}

	plain, err := newToken()
	if err != nil {
		return fmt.Errorf("create: generate token: %w", err)
	}

	env := *environment
	tok := registrationtoken.RegistrationToken{
		TokenHash:   deps.Hasher.Hash(plain),
		Environment: &env,
		ExpiresAt:   time.Now().Add(lifetime),
	}
	if err := deps.Repo.Create(ctx, tok); err != nil {
		return err
	}

	// The plain token is shown exactly once and never stored.
	fmt.Fprintln(deps.Out, "Registration Token")
	fmt.Fprintln(deps.Out)
	fmt.Fprintln(deps.Out, plain)
	return nil
}

// newToken returns a cryptographically secure random token in the documented
// format ops_rt_<base64url(32 random bytes)>.
func newToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// parseLifetime parses the --expires value: a Go duration (24h, 30m, 90s) or a
// whole number of days (7d, 30d). An empty value yields the default lifetime.
func parseLifetime(s string) (time.Duration, error) {
	if s == "" {
		return defaultExpiry, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("create: invalid lifetime %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("create: invalid lifetime %q", s)
	}
	return d, nil
}
