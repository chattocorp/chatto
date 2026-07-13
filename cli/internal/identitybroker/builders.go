package identitybroker

import (
	"crypto/ed25519"
	"fmt"
	"time"
)

// NewGenesisStatement constructs the first certificate statement for two
// founders on distinct server origins.
func NewGenesisStatement(groupID string, founders []Challenge, ceremonyPublicKey ed25519.PublicKey, issuedAt time.Time, validFor time.Duration) (Statement, error) {
	if len(founders) != 2 {
		return Statement{}, fmt.Errorf("%w: genesis requires two founder challenges", ErrInvalidArtifact)
	}
	participants := make([]Participant, 0, 2)
	for _, challenge := range founders {
		if challenge.Kind != KindGenesis || challenge.Role != RoleFounder {
			return Statement{}, ErrChallengeMismatch
		}
		participants = append(participants, participantFromChallenge(challenge))
	}
	statement := Statement{
		Version:           ProtocolVersion,
		Kind:              KindGenesis,
		GroupID:           groupID,
		Participants:      participants,
		CeremonyPublicKey: append([]byte(nil), ceremonyPublicKey...),
		IssuedAt:          issuedAt.Unix(),
		ExpiresAt:         issuedAt.Add(validFor).Unix(),
	}
	return statement, validateAndReturn(statement)
}

// NewMembershipStatement constructs a group join sponsored by two existing
// credentials on distinct server origins.
func NewMembershipStatement(groupID string, target Challenge, sponsors []Challenge, sponsorRefs []SponsorRef, ceremonyPublicKey ed25519.PublicKey, issuedAt time.Time, validFor time.Duration) (Statement, error) {
	if target.Kind != KindMembership || target.Role != RoleTarget || len(sponsors) != 2 || len(sponsorRefs) != 2 {
		return Statement{}, ErrInsufficientSponsors
	}
	participants := []Participant{participantFromChallenge(target)}
	for _, challenge := range sponsors {
		if challenge.Kind != KindMembership || challenge.Role != RoleSponsor {
			return Statement{}, ErrChallengeMismatch
		}
		participants = append(participants, participantFromChallenge(challenge))
	}
	statement := Statement{
		Version:           ProtocolVersion,
		Kind:              KindMembership,
		GroupID:           groupID,
		Subject:           target.Account,
		Sponsors:          append([]SponsorRef(nil), sponsorRefs...),
		Participants:      participants,
		CeremonyPublicKey: append([]byte(nil), ceremonyPublicKey...),
		IssuedAt:          issuedAt.Unix(),
		ExpiresAt:         issuedAt.Add(validFor).Unix(),
	}
	return statement, validateAndReturn(statement)
}

// NewRevocationStatement constructs a member's permanent self-revocation.
func NewRevocationStatement(groupID, credentialID string, member Challenge, ceremonyPublicKey ed25519.PublicKey, issuedAt time.Time) (Statement, error) {
	if member.Kind != KindRevocation || member.Role != RoleMember {
		return Statement{}, ErrChallengeMismatch
	}
	statement := Statement{
		Version:             ProtocolVersion,
		Kind:                KindRevocation,
		GroupID:             groupID,
		Subject:             member.Account,
		RevokedCredentialID: credentialID,
		Participants:        []Participant{participantFromChallenge(member)},
		CeremonyPublicKey:   append([]byte(nil), ceremonyPublicKey...),
		IssuedAt:            issuedAt.Unix(),
	}
	return statement, validateAndReturn(statement)
}

func participantFromChallenge(challenge Challenge) Participant {
	return Participant{
		Role:        challenge.Role,
		Account:     challenge.Account,
		ChallengeID: challenge.ID,
		Nonce:       challenge.Nonce,
	}
}

func validateAndReturn(statement Statement) error {
	return validateStatement(statement)
}
