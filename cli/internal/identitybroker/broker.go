package identitybroker

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultChallengeTTL = 5 * time.Minute
	MaxCredentialTTL    = 30 * 24 * time.Hour
)

type issuedApproval struct {
	statementID string
	approval    Approval
}

// Broker is an in-memory protocol actor for the PoC. Its challenge and
// certificate repositories intentionally model the operations that would move
// to RUNTIME_STATE and EVT, but this type must not be used as production state.
type Broker struct {
	mu sync.Mutex

	origin     string
	keyID      string
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey

	challenges   map[string]Challenge
	approvals    map[string]issuedApproval
	certificates map[string]Certificate
	credentials  map[string]Credential
	revoked      map[string]struct{}
}

// NewBroker creates an isolated in-memory protocol actor and signing identity.
func NewBroker(origin string) (*Broker, error) {
	normalizedOrigin, err := NormalizeOrigin(origin)
	if err != nil {
		return nil, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate broker signing key: %w", err)
	}
	digest := sha256.Sum256(publicKey)
	return &Broker{
		origin:       normalizedOrigin,
		keyID:        base64.RawURLEncoding.EncodeToString(digest[:12]),
		publicKey:    publicKey,
		privateKey:   privateKey,
		challenges:   map[string]Challenge{},
		approvals:    map[string]issuedApproval{},
		certificates: map[string]Certificate{},
		credentials:  map[string]Credential{},
		revoked:      map[string]struct{}{},
	}, nil
}

// Origin returns the broker's canonical server origin.
func (b *Broker) Origin() string {
	return b.origin
}

// Discovery returns the broker's public origin-bound signing metadata.
func (b *Broker) Discovery() DiscoveryKey {
	return DiscoveryKey{
		Protocol:  ProtocolVersion,
		Origin:    b.origin,
		KeyID:     b.keyID,
		PublicKey: append([]byte(nil), b.publicKey...),
	}
}

// IssueChallenge creates one short-lived authorization checkpoint for a local
// authenticated account.
func (b *Broker) IssueChallenge(account Account, kind, role string, ceremonyPublicKey ed25519.PublicKey, now time.Time) (Challenge, error) {
	if account.Origin != b.origin {
		return Challenge{}, fmt.Errorf("%w: account belongs to %s, not %s", ErrChallengeMismatch, account.Origin, b.origin)
	}
	if err := account.Validate(); err != nil {
		return Challenge{}, err
	}
	if !validRoleForKind(kind, role) {
		return Challenge{}, fmt.Errorf("%w: role %q is not valid for %q", ErrChallengeMismatch, role, kind)
	}
	if len(ceremonyPublicKey) != ed25519.PublicKeySize {
		return Challenge{}, fmt.Errorf("%w: ceremony public key has length %d", ErrChallengeMismatch, len(ceremonyPublicKey))
	}
	id, err := NewOpaqueID(24)
	if err != nil {
		return Challenge{}, err
	}
	nonce, err := NewOpaqueID(32)
	if err != nil {
		return Challenge{}, err
	}
	challenge := Challenge{
		ID:                id,
		Nonce:             nonce,
		Kind:              kind,
		Role:              role,
		Account:           account,
		CeremonyPublicKey: append([]byte(nil), ceremonyPublicKey...),
		IssuedAt:          now.Unix(),
		ExpiresAt:         now.Add(DefaultChallengeTTL).Unix(),
	}
	b.mu.Lock()
	b.challenges[id] = cloneChallenge(challenge)
	b.mu.Unlock()
	return cloneChallenge(challenge), nil
}

// Approve validates and consumes the authenticated account's challenge, or
// returns the same approval for an idempotent retry of the same statement. A
// one-party self-revocation is complete at this point and is committed before
// the approval is returned, so the client cannot withhold a later finalization.
func (b *Broker) Approve(authenticated Account, request CeremonyRequest, now time.Time) (Approval, error) {
	if authenticated.Origin != b.origin {
		return Approval{}, ErrChallengeMismatch
	}
	if err := VerifyCeremony(request); err != nil {
		return Approval{}, err
	}

	participant, ok := participantForAccount(request.Statement.Participants, authenticated)
	if !ok {
		return Approval{}, ErrChallengeMismatch
	}
	statementID, err := StatementID(request.Statement)
	if err != nil {
		return Approval{}, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if issued, exists := b.approvals[participant.ChallengeID]; exists {
		if issued.statementID != statementID {
			return Approval{}, ErrApprovalAlreadyIssued
		}
		return cloneApproval(issued.approval), nil
	}
	challenge, exists := b.challenges[participant.ChallengeID]
	if !exists {
		return Approval{}, ErrChallengeNotFound
	}
	if challenge.ExpiresAt < now.Unix() {
		return Approval{}, ErrChallengeExpired
	}
	if challenge.Kind != request.Statement.Kind || challenge.Role != participant.Role || challenge.Account != authenticated || challenge.Nonce != participant.Nonce {
		return Approval{}, ErrChallengeMismatch
	}
	if !ed25519.PublicKey(challenge.CeremonyPublicKey).Equal(ed25519.PublicKey(request.Statement.CeremonyPublicKey)) {
		return Approval{}, fmt.Errorf("%w: ceremony key does not match challenge", ErrChallengeMismatch)
	}
	if request.Statement.IssuedAt < challenge.IssuedAt || request.Statement.IssuedAt > now.Add(time.Minute).Unix() {
		return Approval{}, fmt.Errorf("%w: statement issuance is outside the ceremony window", ErrChallengeMismatch)
	}
	if participant.Role == RoleSponsor {
		for _, sponsor := range request.Statement.Sponsors {
			if sponsor.Account == authenticated {
				if _, revoked := b.revoked[sponsor.CredentialID]; revoked {
					return Approval{}, fmt.Errorf("%w: local sponsor credential is revoked", ErrInsufficientSponsors)
				}
				break
			}
		}
	}
	if request.Statement.Kind == KindRevocation {
		credential, ok := b.credentials[request.Statement.RevokedCredentialID]
		if !ok || credential.GroupID != request.Statement.GroupID || credential.Account != authenticated || !credential.activeAt(now.Unix()) {
			return Approval{}, fmt.Errorf("%w: revocation does not name an active local credential", ErrInvalidArtifact)
		}
	}

	signature, err := signApproval(request.Statement, participant.Role, authenticated, b.privateKey)
	if err != nil {
		return Approval{}, err
	}
	approval := Approval{
		Origin:    b.origin,
		KeyID:     b.keyID,
		Role:      participant.Role,
		Account:   authenticated,
		Signature: signature,
	}
	b.approvals[participant.ChallengeID] = issuedApproval{statementID: statementID, approval: approval}
	delete(b.challenges, participant.ChallengeID)
	if request.Statement.Kind == KindRevocation {
		b.revoked[request.Statement.RevokedCredentialID] = struct{}{}
		credential := b.credentials[request.Statement.RevokedCredentialID]
		credential.RevokedAt = request.Statement.IssuedAt
		b.credentials[credential.ID] = credential
		b.certificates[statementID] = cloneCertificate(Certificate{
			Request:   request,
			Approvals: []Approval{approval},
		})
	}
	return cloneApproval(approval), nil
}

// Finalize verifies and stores a complete certificate involving this broker.
func (b *Broker) Finalize(certificate Certificate, verifier *Verifier, supporting []Certificate, now time.Time) error {
	statementID, err := StatementID(certificate.Request.Statement)
	if err != nil {
		return err
	}
	if !statementContainsOrigin(certificate.Request.Statement, b.origin) {
		return fmt.Errorf("%w: certificate does not involve this broker", ErrInvalidArtifact)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	known := make([]Certificate, 0, len(b.certificates))
	for _, stored := range b.certificates {
		if certificateGroupID(stored) == certificateGroupID(certificate) {
			known = append(known, cloneCertificate(stored))
		}
	}
	bundle := append(known, supporting...)
	bundle = append(bundle, certificate)
	group, err := verifier.Reconstruct(bundle, now)
	if err != nil {
		return err
	}
	if existing, ok := b.certificates[statementID]; ok {
		existingID, existingErr := StatementID(existing.Request.Statement)
		if existingErr != nil || existingID != statementID {
			return fmt.Errorf("%w: conflicting finalized certificate", ErrInvalidArtifact)
		}
		return nil
	}
	if certificate.Request.Statement.Kind == KindRevocation {
		b.revoked[certificate.Request.Statement.RevokedCredentialID] = struct{}{}
	}
	for id, credential := range group.Credentials {
		b.credentials[id] = credential
	}
	b.certificates[statementID] = cloneCertificate(certificate)
	return nil
}

// Certificates returns defensive copies of the broker's finalized artifacts.
func (b *Broker) Certificates() []Certificate {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]Certificate, 0, len(b.certificates))
	for _, certificate := range b.certificates {
		result = append(result, cloneCertificate(certificate))
	}
	return result
}

func participantForAccount(participants []Participant, account Account) (Participant, bool) {
	for _, participant := range participants {
		if participant.Account == account {
			return participant, true
		}
	}
	return Participant{}, false
}

func validRoleForKind(kind, role string) bool {
	switch kind {
	case KindGenesis:
		return role == RoleFounder
	case KindMembership:
		return role == RoleTarget || role == RoleSponsor
	case KindRevocation:
		return role == RoleMember
	default:
		return false
	}
}

func statementContainsOrigin(statement Statement, origin string) bool {
	for _, participant := range statement.Participants {
		if participant.Account.Origin == origin {
			return true
		}
	}
	return false
}

func cloneApproval(approval Approval) Approval {
	approval.Signature = append([]byte(nil), approval.Signature...)
	return approval
}

func cloneChallenge(challenge Challenge) Challenge {
	challenge.CeremonyPublicKey = append([]byte(nil), challenge.CeremonyPublicKey...)
	return challenge
}

func certificateGroupID(certificate Certificate) string {
	if certificate.Request.Statement.Kind != KindGenesis {
		return certificate.Request.Statement.GroupID
	}
	id, _ := StatementID(certificate.Request.Statement)
	return id
}

func cloneCertificate(certificate Certificate) Certificate {
	certificate.Request.Statement.CeremonyPublicKey = append([]byte(nil), certificate.Request.Statement.CeremonyPublicKey...)
	certificate.Request.Statement.Sponsors = append([]SponsorRef(nil), certificate.Request.Statement.Sponsors...)
	certificate.Request.Statement.Participants = append([]Participant(nil), certificate.Request.Statement.Participants...)
	certificate.Request.CeremonySignature = append([]byte(nil), certificate.Request.CeremonySignature...)
	certificate.Approvals = append([]Approval(nil), certificate.Approvals...)
	for i := range certificate.Approvals {
		certificate.Approvals[i] = cloneApproval(certificate.Approvals[i])
	}
	return certificate
}
