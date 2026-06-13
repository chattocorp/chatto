//go:build test_endpoints

package core

import "golang.org/x/crypto/bcrypt"

// Test-endpoint builds create many short-lived users during E2E runs. Keep the
// auth flow real while avoiding production-grade bcrypt cost for throwaway data.
const passwordHashCost = bcrypt.MinCost
