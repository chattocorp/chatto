package core

import (
	"sync"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// PasskeyProjection owns credential-hash lookup state. Credential material is
// intentionally not duplicated in UserProjection: passkey assertions update it
// frequently and must not contend with unrelated account mutations.
type PasskeyProjection struct {
	events.MemoryProjection
	sync.RWMutex
	credentials map[string]Passkey
}

func NewPasskeyProjection() *PasskeyProjection {
	return &PasskeyProjection{credentials: make(map[string]Passkey)}
}

func (p *PasskeyProjection) Subjects() []string { return []string{events.PasskeySubjectFilter()} }

func (p *PasskeyProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *corev1.Event_PasskeyCredentialRegistered:
		v := e.PasskeyCredentialRegistered
		if v != nil && v.GetCredentialHash() != "" && v.GetUserId() != "" && len(v.GetCredentialId()) > 0 && len(v.GetCredential()) > 0 {
			p.credentials[v.GetCredentialHash()] = Passkey{UserID: v.GetUserId(), CredentialHash: v.GetCredentialHash(), CredentialID: append([]byte(nil), v.GetCredentialId()...), Credential: append([]byte(nil), v.GetCredential()...)}
		}
	case *corev1.Event_PasskeyCredentialUpdated:
		v := e.PasskeyCredentialUpdated
		if v != nil && len(v.GetCredential()) > 0 {
			if existing, ok := p.credentials[v.GetCredentialHash()]; ok {
				existing.Credential = append([]byte(nil), v.GetCredential()...)
				p.credentials[v.GetCredentialHash()] = existing
			}
		}
	case *corev1.Event_PasskeyCredentialRemoved:
		if v := e.PasskeyCredentialRemoved; v != nil {
			delete(p.credentials, v.GetCredentialHash())
		}
	}
	return nil
}

func (p *PasskeyProjection) Get(credentialHash string) (Passkey, bool) {
	p.RLock()
	defer p.RUnlock()
	credential, ok := p.credentials[credentialHash]
	if !ok {
		return Passkey{}, false
	}
	credential.CredentialID = append([]byte(nil), credential.CredentialID...)
	credential.Credential = append([]byte(nil), credential.Credential...)
	return credential, true
}
