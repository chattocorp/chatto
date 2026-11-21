package identitybroker

import (
	"crypto/ed25519"
	"fmt"
	"slices"
	"strings"
	"time"
)

func validateStatement(statement Statement) error {
	if statement.Version != ProtocolVersion {
		return fmt.Errorf("%w: unsupported protocol version %q", ErrInvalidArtifact, statement.Version)
	}
	if statement.Kind != KindGenesis && strings.TrimSpace(statement.GroupID) == "" {
		return fmt.Errorf("%w: group id is empty", ErrInvalidArtifact)
	}
	if len(statement.CeremonyPublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: ceremony public key has length %d", ErrInvalidArtifact, len(statement.CeremonyPublicKey))
	}
	if statement.IssuedAt <= 0 {
		return fmt.Errorf("%w: issued-at is missing", ErrInvalidArtifact)
	}
	if statement.Kind != KindRevocation && statement.ExpiresAt <= statement.IssuedAt {
		return fmt.Errorf("%w: credential expiry must follow issuance", ErrInvalidArtifact)
	}
	if statement.Kind != KindRevocation && statement.ExpiresAt-statement.IssuedAt > int64(MaxCredentialTTL/time.Second) {
		return fmt.Errorf("%w: credential lifetime exceeds %s", ErrInvalidArtifact, MaxCredentialTTL)
	}

	participants := append([]Participant(nil), statement.Participants...)
	sortParticipants(participants)
	for i, participant := range participants {
		if err := participant.Account.Validate(); err != nil {
			return err
		}
		if participant.ChallengeID == "" || participant.Nonce == "" {
			return fmt.Errorf("%w: participant challenge is incomplete", ErrInvalidArtifact)
		}
		if i > 0 && participant.Role == participants[i-1].Role && participant.Account == participants[i-1].Account {
			return fmt.Errorf("%w: duplicate participant", ErrInvalidArtifact)
		}
	}

	switch statement.Kind {
	case KindGenesis:
		return validateGenesisStatement(statement)
	case KindMembership:
		return validateMembershipStatement(statement)
	case KindRevocation:
		return validateRevocationStatement(statement)
	default:
		return fmt.Errorf("%w: unknown statement kind %q", ErrInvalidArtifact, statement.Kind)
	}
}

func validateGenesisStatement(statement Statement) error {
	if statement.GroupID != "" {
		return fmt.Errorf("%w: genesis group id must be derived from its certificate", ErrInvalidArtifact)
	}
	if statement.Subject != (Account{}) || statement.RevokedCredentialID != "" || len(statement.Sponsors) != 0 {
		return fmt.Errorf("%w: genesis contains membership or revocation fields", ErrInvalidArtifact)
	}
	if len(statement.Participants) != 2 {
		return fmt.Errorf("%w: genesis requires two founders", ErrInvalidArtifact)
	}
	origins := map[string]struct{}{}
	for _, participant := range statement.Participants {
		if participant.Role != RoleFounder {
			return fmt.Errorf("%w: genesis participant is not a founder", ErrInvalidArtifact)
		}
		origins[participant.Account.Origin] = struct{}{}
	}
	if len(origins) != 2 {
		return fmt.Errorf("%w: founders must be on distinct servers", ErrInvalidArtifact)
	}
	return nil
}

func validateMembershipStatement(statement Statement) error {
	if err := statement.Subject.Validate(); err != nil {
		return err
	}
	if statement.RevokedCredentialID != "" {
		return fmt.Errorf("%w: membership revokes a credential", ErrInvalidArtifact)
	}
	if len(statement.Sponsors) != 2 || len(statement.Participants) != 3 {
		return ErrInsufficientSponsors
	}

	sponsors := append([]SponsorRef(nil), statement.Sponsors...)
	sortSponsors(sponsors)
	if sponsors[0].Account == sponsors[1].Account || sponsors[0].Account.Origin == sponsors[1].Account.Origin {
		return ErrInsufficientSponsors
	}
	for _, sponsor := range sponsors {
		if err := sponsor.Account.Validate(); err != nil {
			return err
		}
		if sponsor.CredentialID == "" {
			return fmt.Errorf("%w: sponsor credential id is empty", ErrInvalidArtifact)
		}
	}

	targets := 0
	participantSponsors := make([]Account, 0, 2)
	seenOrigins := map[string]struct{}{}
	for _, participant := range statement.Participants {
		if _, exists := seenOrigins[participant.Account.Origin]; exists {
			return fmt.Errorf("%w: every membership participant must use a distinct server", ErrInvalidArtifact)
		}
		seenOrigins[participant.Account.Origin] = struct{}{}
		switch participant.Role {
		case RoleTarget:
			targets++
			if participant.Account != statement.Subject {
				return fmt.Errorf("%w: target participant does not match subject", ErrInvalidArtifact)
			}
		case RoleSponsor:
			participantSponsors = append(participantSponsors, participant.Account)
		default:
			return fmt.Errorf("%w: unexpected membership role %q", ErrInvalidArtifact, participant.Role)
		}
	}
	if targets != 1 || len(participantSponsors) != 2 {
		return ErrInsufficientSponsors
	}
	slices.SortFunc(participantSponsors, func(a, b Account) int { return strings.Compare(a.key(), b.key()) })
	if participantSponsors[0] != sponsors[0].Account || participantSponsors[1] != sponsors[1].Account {
		return fmt.Errorf("%w: sponsor participants do not match sponsor references", ErrInvalidArtifact)
	}
	return nil
}

func validateRevocationStatement(statement Statement) error {
	if err := statement.Subject.Validate(); err != nil {
		return err
	}
	if statement.RevokedCredentialID == "" || len(statement.Sponsors) != 0 || statement.ExpiresAt != 0 {
		return fmt.Errorf("%w: malformed revocation", ErrInvalidArtifact)
	}
	if len(statement.Participants) != 1 {
		return fmt.Errorf("%w: revocation requires the affected member", ErrInvalidArtifact)
	}
	participant := statement.Participants[0]
	if participant.Role != RoleMember || participant.Account != statement.Subject {
		return fmt.Errorf("%w: revocation participant does not match member", ErrInvalidArtifact)
	}
	return nil
}
