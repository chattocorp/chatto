package identitybroker

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const maxCanonicalFieldLength = 1 << 20

type canonicalEncoder struct {
	bytes.Buffer
}

func (e *canonicalEncoder) string(value string) error {
	return e.bytes([]byte(value))
}

func (e *canonicalEncoder) bytes(value []byte) error {
	if len(value) > maxCanonicalFieldLength {
		return fmt.Errorf("%w: canonical field exceeds %d bytes", ErrInvalidArtifact, maxCanonicalFieldLength)
	}
	if err := binary.Write(&e.Buffer, binary.BigEndian, uint32(len(value))); err != nil {
		return err
	}
	_, err := e.Write(value)
	return err
}

func (e *canonicalEncoder) int64(value int64) error {
	return binary.Write(&e.Buffer, binary.BigEndian, value)
}

func (e *canonicalEncoder) account(account Account) error {
	if err := e.string(account.Origin); err != nil {
		return err
	}
	return e.string(account.UserID)
}

func canonicalStatement(statement Statement) ([]byte, error) {
	if err := validateStatement(statement); err != nil {
		return nil, err
	}

	participants := append([]Participant(nil), statement.Participants...)
	sponsors := append([]SponsorRef(nil), statement.Sponsors...)
	sortParticipants(participants)
	sortSponsors(sponsors)

	var e canonicalEncoder
	for _, value := range []string{
		"chatto-identity-broker-statement-v1",
		statement.Version,
		statement.Kind,
		statement.GroupID,
	} {
		if err := e.string(value); err != nil {
			return nil, err
		}
	}
	if err := e.account(statement.Subject); err != nil {
		return nil, err
	}
	if err := e.string(statement.RevokedCredentialID); err != nil {
		return nil, err
	}
	if err := e.int64(int64(len(sponsors))); err != nil {
		return nil, err
	}
	for _, sponsor := range sponsors {
		if err := e.account(sponsor.Account); err != nil {
			return nil, err
		}
		if err := e.string(sponsor.CredentialID); err != nil {
			return nil, err
		}
	}
	if err := e.int64(int64(len(participants))); err != nil {
		return nil, err
	}
	for _, participant := range participants {
		if err := e.string(participant.Role); err != nil {
			return nil, err
		}
		if err := e.account(participant.Account); err != nil {
			return nil, err
		}
		if err := e.string(participant.ChallengeID); err != nil {
			return nil, err
		}
		if err := e.string(participant.Nonce); err != nil {
			return nil, err
		}
	}
	if err := e.bytes(statement.CeremonyPublicKey); err != nil {
		return nil, err
	}
	if err := e.int64(statement.IssuedAt); err != nil {
		return nil, err
	}
	if err := e.int64(statement.ExpiresAt); err != nil {
		return nil, err
	}
	return e.Bytes(), nil
}

func approvalSigningBytes(statementID, role string, account Account) ([]byte, error) {
	var e canonicalEncoder
	for _, value := range []string{
		"chatto-identity-broker-approval-v1",
		statementID,
		role,
	} {
		if err := e.string(value); err != nil {
			return nil, err
		}
	}
	if err := e.account(account); err != nil {
		return nil, err
	}
	return e.Bytes(), nil
}

func ceremonySigningBytes(statementID string) ([]byte, error) {
	var e canonicalEncoder
	if err := e.string("chatto-identity-broker-ceremony-v1"); err != nil {
		return nil, err
	}
	if err := e.string(statementID); err != nil {
		return nil, err
	}
	return e.Bytes(), nil
}
