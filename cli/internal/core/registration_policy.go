package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// RegistrationAdmission reads the effective self-service account policy.
// No policy event means open. Unknown persisted values remain intact.
func (cm *ConfigModel) RegistrationAdmission() evtv1.RegistrationAdmission {
	p := cm.config.Projection()
	p.RLock()
	defer p.RUnlock()
	return effectiveRegistrationAdmission(p.server.registrationAdmission)
}

// effectiveRegistrationAdmission resolves the default without accepting unknown policies.
func effectiveRegistrationAdmission(admission evtv1.RegistrationAdmission) evtv1.RegistrationAdmission {
	if admission == evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_UNSPECIFIED {
		return evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN
	}
	return admission
}

// InvitationRequired is a UI/early-validation hint. Final admission is checked
// under account creation's stream-wide OCC. Unknown values fail closed.
func (c *ChattoCore) InvitationRequired() bool {
	return c.ConfigModel().RegistrationAdmission() != evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN
}

// WaitForRegistrationPolicy catches this replica up before an admission read.
func (c *ChattoCore) WaitForRegistrationPolicy(ctx context.Context) error {
	return c.ConfigModel().config.Projector().WaitForCurrent(ctx)
}
