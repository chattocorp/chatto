package identitybroker

import (
	"crypto/ed25519"
	"fmt"
)

// SignCeremony binds a statement to one disposable ceremony key.
func SignCeremony(statement Statement, privateKey ed25519.PrivateKey) (CeremonyRequest, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return CeremonyRequest{}, fmt.Errorf("%w: ceremony private key has length %d", ErrInvalidArtifact, len(privateKey))
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if !publicKey.Equal(ed25519.PublicKey(statement.CeremonyPublicKey)) {
		return CeremonyRequest{}, fmt.Errorf("%w: ceremony private key does not match statement", ErrInvalidArtifact)
	}
	statementID, err := StatementID(statement)
	if err != nil {
		return CeremonyRequest{}, err
	}
	payload, err := ceremonySigningBytes(statementID)
	if err != nil {
		return CeremonyRequest{}, err
	}
	return CeremonyRequest{
		Statement:         statement,
		CeremonySignature: ed25519.Sign(privateKey, payload),
	}, nil
}

// VerifyCeremony validates the disposable client's proof of possession.
func VerifyCeremony(request CeremonyRequest) error {
	statementID, err := StatementID(request.Statement)
	if err != nil {
		return err
	}
	payload, err := ceremonySigningBytes(statementID)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(request.Statement.CeremonyPublicKey), payload, request.CeremonySignature) {
		return ErrInvalidSignature
	}
	return nil
}

func signApproval(statement Statement, role string, account Account, privateKey ed25519.PrivateKey) ([]byte, error) {
	statementID, err := StatementID(statement)
	if err != nil {
		return nil, err
	}
	payload, err := approvalSigningBytes(statementID, role, account)
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(privateKey, payload), nil
}

func verifyApproval(statement Statement, approval Approval, publicKey ed25519.PublicKey) error {
	statementID, err := StatementID(statement)
	if err != nil {
		return err
	}
	payload, err := approvalSigningBytes(statementID, approval.Role, approval.Account)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, payload, approval.Signature) {
		return ErrInvalidSignature
	}
	return nil
}
