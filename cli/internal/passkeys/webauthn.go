// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package passkeys centralizes WebAuthn relying-party construction. Credential
// persistence and ceremony lifecycle stay in core services; this package keeps
// security-sensitive RP policy identical for every caller.
package passkeys

import (
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"hmans.de/chatto/internal/config"
)

// New creates Chatto's WebAuthn relying-party verifier from the validated
// public server URL. It accepts any authenticator attachment, requires a local
// user-verification gesture, creates discoverable credentials, and asks no
// authenticator to reveal attestation.
func New(cfg config.ChattoConfig, displayName string) (*webauthn.WebAuthn, error) {
	rpID, origin, ok := cfg.PasskeyRelyingParty()
	if !ok {
		return nil, fmt.Errorf("passkeys are disabled or have no valid public relying-party URL")
	}
	if displayName == "" {
		displayName = "Chatto"
	}
	return webauthn.New(&webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         displayName,
		RPOrigins:             []string{origin},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
	})
}
