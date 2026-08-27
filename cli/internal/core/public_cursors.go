// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import "hmans.de/chatto/internal/publiccursor"

// SealPublicCursor protects an internal API coordinate with the server-wide
// secret. Purpose and scope must be stable, domain-specific values.
func (c *ChattoCore) SealPublicCursor(purpose, scope string, payload []byte) (string, error) {
	return publiccursor.Seal(c.config.SecretKey, purpose, scope, payload)
}

// OpenPublicCursor authenticates and decrypts a cursor created by
// SealPublicCursor.
func (c *ChattoCore) OpenPublicCursor(purpose, scope, token string) ([]byte, error) {
	return publiccursor.Open(c.config.SecretKey, purpose, scope, token)
}
