package identitybroker

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

const (
	ProtocolVersion = "chatto-identity-broker-poc-v1"

	KindGenesis    = "genesis"
	KindMembership = "membership"
	KindRevocation = "revocation"

	RoleFounder = "founder"
	RoleTarget  = "target"
	RoleSponsor = "sponsor"
	RoleMember  = "member"
)

var (
	ErrInvalidArtifact       = errors.New("identity broker: invalid artifact")
	ErrInvalidSignature      = errors.New("identity broker: invalid signature")
	ErrChallengeNotFound     = errors.New("identity broker: challenge not found")
	ErrChallengeExpired      = errors.New("identity broker: challenge expired")
	ErrChallengeMismatch     = errors.New("identity broker: challenge mismatch")
	ErrApprovalAlreadyIssued = errors.New("identity broker: approval already issued for different statement")
	ErrCertificateIncomplete = errors.New("identity broker: certificate is incomplete")
	ErrInsufficientSponsors  = errors.New("identity broker: membership requires two current sponsors on distinct servers")
)

// Account is one server-scoped Chatto account. UserID is an opaque stable ID;
// mutable usernames and profile fields are intentionally not identity inputs.
type Account struct {
	Origin string `json:"origin"`
	UserID string `json:"user_id"`
}

func (a Account) Validate() error {
	normalizedOrigin, err := NormalizeOrigin(a.Origin)
	if err != nil {
		return fmt.Errorf("%w: account origin: %v", ErrInvalidArtifact, err)
	}
	if normalizedOrigin != a.Origin {
		return fmt.Errorf("%w: account origin is not canonical", ErrInvalidArtifact)
	}
	if strings.TrimSpace(a.UserID) == "" {
		return fmt.Errorf("%w: account user id is empty", ErrInvalidArtifact)
	}
	return nil
}

func (a Account) key() string {
	return a.Origin + "\x00" + a.UserID
}

// Challenge is a short-lived, single-server authorization checkpoint. A
// production implementation would persist it in RUNTIME_STATE with a TTL.
type Challenge struct {
	ID                string  `json:"id"`
	Nonce             string  `json:"nonce"`
	Kind              string  `json:"kind"`
	Role              string  `json:"role"`
	Account           Account `json:"account"`
	CeremonyPublicKey []byte  `json:"ceremony_public_key"`
	IssuedAt          int64   `json:"issued_at"`
	ExpiresAt         int64   `json:"expires_at"`
}

// Participant binds one expected server approval to the exact challenge that
// the authenticated account authorized.
type Participant struct {
	Role        string  `json:"role"`
	Account     Account `json:"account"`
	ChallengeID string  `json:"challenge_id"`
	Nonce       string  `json:"nonce"`
}

// SponsorRef proves which already-issued group credential authorized a new
// member. Two distinct sponsor origins prevent one malicious existing server
// from extending a group by itself.
type SponsorRef struct {
	Account      Account `json:"account"`
	CredentialID string  `json:"credential_id"`
}

// Statement is the canonical signed payload for all PoC certificate kinds.
// Exact validation depends on Kind.
type Statement struct {
	Version             string        `json:"version"`
	Kind                string        `json:"kind"`
	GroupID             string        `json:"group_id"`
	Subject             Account       `json:"subject,omitempty"`
	RevokedCredentialID string        `json:"revoked_credential_id,omitempty"`
	Sponsors            []SponsorRef  `json:"sponsors,omitempty"`
	Participants        []Participant `json:"participants"`
	CeremonyPublicKey   []byte        `json:"ceremony_public_key"`
	IssuedAt            int64         `json:"issued_at"`
	ExpiresAt           int64         `json:"expires_at,omitempty"`
}

// CeremonyRequest is presented independently to every participating server.
// Every challenge commits to the disposable key before disclosing its nonce,
// so an intercepted request cannot be completed with a substituted client key.
type CeremonyRequest struct {
	Statement         Statement `json:"statement"`
	CeremonySignature []byte    `json:"ceremony_signature"`
}

// Approval is one origin's signature for one account/role in a statement.
// The server attests that it freshly authenticated the named local account.
type Approval struct {
	Origin    string  `json:"origin"`
	KeyID     string  `json:"key_id"`
	Role      string  `json:"role"`
	Account   Account `json:"account"`
	Signature []byte  `json:"signature"`
}

// Certificate combines a ceremony request with every required server
// approval. It contains no bearer token or persistent client secret.
type Certificate struct {
	Request   CeremonyRequest `json:"request"`
	Approvals []Approval      `json:"approvals"`
}

// DiscoveryKey is the origin-bound public signing metadata used by verifiers.
type DiscoveryKey struct {
	Protocol  string `json:"protocol"`
	Origin    string `json:"origin"`
	KeyID     string `json:"key_id"`
	PublicKey []byte `json:"public_key"`
}

// NewOpaqueID returns a base64url-encoded identifier backed by cryptographic
// randomness.
func NewOpaqueID(byteCount int) (string, error) {
	if byteCount < 16 {
		return "", fmt.Errorf("%w: opaque ids require at least 16 random bytes", ErrInvalidArtifact)
	}
	b := make([]byte, byteCount)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate opaque id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewCeremonyKey creates a disposable proof-of-possession key for one ceremony.
func NewCeremonyKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ceremony key: %w", err)
	}
	return publicKey, privateKey, nil
}

// StatementID hashes the validated canonical statement bytes.
func StatementID(statement Statement) (string, error) {
	payload, err := canonicalStatement(statement)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

// CredentialID derives one account-specific credential ID from a certificate.
func CredentialID(certificateID string, account Account) string {
	h := sha256.New()
	h.Write([]byte("chatto-identity-broker-credential-v1\x00"))
	h.Write([]byte(certificateID))
	h.Write([]byte{0})
	h.Write([]byte(account.Origin))
	h.Write([]byte{0})
	h.Write([]byte(account.UserID))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// NormalizeOrigin returns an origin containing only a lowercase scheme and
// host. Production origins require HTTPS; HTTP is accepted only for loopback
// protocol tests.
func NormalizeOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	if u.Scheme == "" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("origin must contain only scheme and host")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	if scheme != "https" {
		hostname := u.Hostname()
		ip := net.ParseIP(hostname)
		if scheme != "http" || (hostname != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return "", errors.New("origin must use https except for loopback tests")
		}
	}
	return scheme + "://" + host, nil
}

func sortParticipants(participants []Participant) {
	slices.SortFunc(participants, func(a, b Participant) int {
		if c := strings.Compare(a.Role, b.Role); c != 0 {
			return c
		}
		return strings.Compare(a.Account.key(), b.Account.key())
	})
}

func sortSponsors(sponsors []SponsorRef) {
	slices.SortFunc(sponsors, func(a, b SponsorRef) int {
		return strings.Compare(a.Account.key(), b.Account.key())
	})
}
