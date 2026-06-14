//go:build !test_endpoints

package core

import "golang.org/x/crypto/bcrypt"

const passwordHashCost = bcrypt.DefaultCost
