// Package ids creates opaque Authling identifiers.
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
)

var encoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// New returns an opaque identifier with a non-sensitive type prefix.
func New(prefix string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + encoding.EncodeToString(random), nil
}
