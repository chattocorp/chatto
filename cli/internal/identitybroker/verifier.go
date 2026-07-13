package identitybroker

import (
	"cmp"
	"crypto/ed25519"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Credential is one account's independently verifiable membership in a group.
type Credential struct {
	ID            string
	GroupID       string
	Account       Account
	CertificateID string
	IssuedAt      int64
	ExpiresAt     int64
	RevokedAt     int64
}

func (c Credential) activeAt(unixTime int64) bool {
	return c.IssuedAt <= unixTime && unixTime < c.ExpiresAt && (c.RevokedAt == 0 || unixTime < c.RevokedAt)
}

// Group is the verified view reconstructed from an unordered certificate
// bundle. Members contains only credentials active at the requested time.
type Group struct {
	ID          string
	GenesisID   string
	Members     map[string]Account
	Credentials map[string]Credential
}

type Verifier struct {
	trust *TrustStore
}

// NewVerifier creates a certificate verifier using pinned origin keys.
func NewVerifier(trust *TrustStore) *Verifier {
	return &Verifier{trust: trust}
}

type verifiedCertificate struct {
	id          string
	certificate Certificate
}

// VerifyCertificate validates structure, ceremony proof, and the exact set of
// server approvals. It returns the canonical statement ID.
func (v *Verifier) VerifyCertificate(certificate Certificate) (string, error) {
	if err := VerifyCeremony(certificate.Request); err != nil {
		return "", err
	}
	statement := certificate.Request.Statement
	statementID, err := StatementID(statement)
	if err != nil {
		return "", err
	}
	if len(certificate.Approvals) != len(statement.Participants) {
		return "", ErrCertificateIncomplete
	}

	approvals := make(map[string]Approval, len(certificate.Approvals))
	for _, approval := range certificate.Approvals {
		if approval.Origin != approval.Account.Origin {
			return "", fmt.Errorf("%w: approval origin does not match account", ErrInvalidArtifact)
		}
		key := approval.Role + "\x00" + approval.Account.key()
		if _, exists := approvals[key]; exists {
			return "", fmt.Errorf("%w: duplicate approval", ErrInvalidArtifact)
		}
		publicKey, ok := v.trust.key(approval.Origin, approval.KeyID)
		if !ok {
			return "", fmt.Errorf("%w: untrusted key %q for %s", ErrInvalidSignature, approval.KeyID, approval.Origin)
		}
		if err := verifyApproval(statement, approval, ed25519.PublicKey(publicKey)); err != nil {
			return "", err
		}
		approvals[key] = approval
	}
	for _, participant := range statement.Participants {
		key := participant.Role + "\x00" + participant.Account.key()
		if _, ok := approvals[key]; !ok {
			return "", ErrCertificateIncomplete
		}
	}
	return statementID, nil
}

// Reconstruct verifies an unordered bundle and returns its single identity
// group. Membership dependencies are resolved by credential ID, not input
// order or server-provided timestamps.
func (v *Verifier) Reconstruct(certificates []Certificate, now time.Time) (*Group, error) {
	if len(certificates) == 0 {
		return nil, fmt.Errorf("%w: certificate bundle is empty", ErrInvalidArtifact)
	}
	records := make([]verifiedCertificate, 0, len(certificates))
	seenIDs := map[string]struct{}{}
	groupID := ""
	for _, certificate := range certificates {
		certificateID, err := v.VerifyCertificate(certificate)
		if err != nil {
			return nil, err
		}
		if _, exists := seenIDs[certificateID]; exists {
			continue
		}
		seenIDs[certificateID] = struct{}{}
		statement := certificate.Request.Statement
		if groupID == "" {
			groupID = statement.GroupID
		} else if statement.GroupID != groupID {
			return nil, fmt.Errorf("%w: bundle contains multiple groups", ErrInvalidArtifact)
		}
		records = append(records, verifiedCertificate{id: certificateID, certificate: certificate})
	}

	group := &Group{
		ID:          groupID,
		Members:     map[string]Account{},
		Credentials: map[string]Credential{},
	}

	genesisCount := 0
	pendingMemberships := make([]verifiedCertificate, 0)
	revocations := make([]verifiedCertificate, 0)
	for _, record := range records {
		statement := record.certificate.Request.Statement
		switch statement.Kind {
		case KindGenesis:
			genesisCount++
			group.GenesisID = record.id
			for _, participant := range statement.Participants {
				credential := Credential{
					ID:            CredentialID(record.id, participant.Account),
					GroupID:       groupID,
					Account:       participant.Account,
					CertificateID: record.id,
					IssuedAt:      statement.IssuedAt,
					ExpiresAt:     statement.ExpiresAt,
				}
				group.Credentials[credential.ID] = credential
			}
		case KindMembership:
			pendingMemberships = append(pendingMemberships, record)
		case KindRevocation:
			revocations = append(revocations, record)
		}
	}
	if genesisCount != 1 {
		return nil, fmt.Errorf("%w: bundle requires exactly one genesis certificate", ErrInvalidArtifact)
	}

	for len(pendingMemberships) > 0 {
		progress := false
		remaining := pendingMemberships[:0]
		for _, record := range pendingMemberships {
			statement := record.certificate.Request.Statement
			if !sponsorsResolveAt(group.Credentials, statement.Sponsors, statement.IssuedAt) {
				remaining = append(remaining, record)
				continue
			}
			credential := Credential{
				ID:            CredentialID(record.id, statement.Subject),
				GroupID:       groupID,
				Account:       statement.Subject,
				CertificateID: record.id,
				IssuedAt:      statement.IssuedAt,
				ExpiresAt:     statement.ExpiresAt,
			}
			group.Credentials[credential.ID] = credential
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("%w: membership sponsor chain is unresolved", ErrInsufficientSponsors)
		}
		pendingMemberships = remaining
	}

	slices.SortFunc(revocations, func(a, b verifiedCertificate) int {
		return cmp.Compare(a.certificate.Request.Statement.IssuedAt, b.certificate.Request.Statement.IssuedAt)
	})
	for _, record := range revocations {
		statement := record.certificate.Request.Statement
		credential, ok := group.Credentials[statement.RevokedCredentialID]
		if !ok || credential.Account != statement.Subject || !credential.activeAt(statement.IssuedAt) {
			return nil, fmt.Errorf("%w: revocation does not name an active subject credential", ErrInvalidArtifact)
		}
		credential.RevokedAt = statement.IssuedAt
		group.Credentials[credential.ID] = credential
	}

	// A sponsor revoked before a membership was issued could not authorize that
	// membership, even when the dependency graph itself resolves.
	for _, record := range records {
		statement := record.certificate.Request.Statement
		if statement.Kind == KindMembership && !sponsorsResolveAt(group.Credentials, statement.Sponsors, statement.IssuedAt) {
			return nil, fmt.Errorf("%w: membership used an inactive sponsor", ErrInsufficientSponsors)
		}
	}

	activeCredentials := map[string]string{}
	for _, credential := range group.Credentials {
		if credential.activeAt(now.Unix()) {
			accountKey := credential.Account.key()
			if previousCredentialID, exists := activeCredentials[accountKey]; exists {
				return nil, fmt.Errorf("%w: account has multiple active credentials (%s and %s)", ErrInvalidArtifact, previousCredentialID, credential.ID)
			}
			activeCredentials[accountKey] = credential.ID
			group.Members[accountKey] = credential.Account
		}
	}
	return group, nil
}

func sponsorsResolveAt(credentials map[string]Credential, sponsors []SponsorRef, at int64) bool {
	if len(sponsors) != 2 {
		return false
	}
	origins := map[string]struct{}{}
	for _, sponsor := range sponsors {
		credential, ok := credentials[sponsor.CredentialID]
		if !ok || credential.Account != sponsor.Account || !credential.activeAt(at) {
			return false
		}
		origins[credential.Account.Origin] = struct{}{}
	}
	return len(origins) == 2
}

// MemberAccounts returns active accounts in stable origin/user-ID order.
func (g *Group) MemberAccounts() []Account {
	accounts := make([]Account, 0, len(g.Members))
	for _, account := range g.Members {
		accounts = append(accounts, account)
	}
	slices.SortFunc(accounts, func(a, b Account) int { return strings.Compare(a.key(), b.key()) })
	return accounts
}
