//go:build bootstrap

package accounts

import (
	"context"
	"errors"
	"fmt"
)

// BootstrapDevelopmentUsers creates the fixed, verified local test accounts.
// It runs only after replay, before HTTP startup. Existing email claims are
// preserved, including passwords, profiles, IDs, and administrator flags.
// Deleted or renamed fixture addresses are created again on the next start.
// The normal service policy is unchanged: only this copy accepts the short
// fixture password. This method and its credentials do not exist in releases.
func (s *Service) BootstrapDevelopmentUsers(ctx context.Context) error {
	bootstrap := *s
	bootstrap.passwordMinimumLength = 8
	for _, email := range []string{"alice@alice.com", "bob@bob.com"} {
		if s.HasEmail(email) {
			continue
		}
		if _, err := bootstrap.CreateLocal(ctx, email, "foobar123"); err != nil && !errors.Is(err, ErrEmailClaimed) {
			// Do not include credentials or identifiers in startup errors.
			return fmt.Errorf("create development account: %w", err)
		}
	}
	return nil
}
