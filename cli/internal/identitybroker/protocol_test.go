package identitybroker

import (
	"crypto/ed25519"
	"errors"
	"reflect"
	"testing"
	"time"
)

var testNow = time.Unix(1_800_000_000, 0).UTC()

type testMember struct {
	broker  *Broker
	account Account
}

func newTestMember(t *testing.T, origin, userID string) testMember {
	t.Helper()
	broker, err := NewBroker(origin)
	if err != nil {
		t.Fatalf("NewBroker(%q): %v", origin, err)
	}
	return testMember{broker: broker, account: Account{Origin: broker.Origin(), UserID: userID}}
}

func trustMembers(t *testing.T, members ...testMember) *TrustStore {
	t.Helper()
	trust := NewTrustStore()
	for _, member := range members {
		if err := trust.Add(member.account.Origin, member.broker.Discovery()); err != nil {
			t.Fatalf("trust.Add(%s): %v", member.account.Origin, err)
		}
	}
	return trust
}

func issueChallenge(t *testing.T, member testMember, kind, role string, ceremonyPublicKey ed25519.PublicKey, now time.Time) Challenge {
	t.Helper()
	challenge, err := member.broker.IssueChallenge(member.account, kind, role, ceremonyPublicKey, now)
	if err != nil {
		t.Fatalf("IssueChallenge(%s, %s): %v", kind, role, err)
	}
	return challenge
}

func signWithMembers(t *testing.T, statement Statement, privateKey ed25519.PrivateKey, now time.Time, members ...testMember) Certificate {
	t.Helper()
	request, err := SignCeremony(statement, privateKey)
	if err != nil {
		t.Fatalf("SignCeremony: %v", err)
	}
	certificate := Certificate{Request: request}
	for _, member := range members {
		approval, err := member.broker.Approve(member.account, request, now)
		if err != nil {
			t.Fatalf("Approve(%s): %v", member.account.Origin, err)
		}
		certificate.Approvals = append(certificate.Approvals, approval)
	}
	return certificate
}

func makeGenesis(t *testing.T, first, second testMember, now time.Time) (Certificate, string) {
	t.Helper()
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatalf("NewCeremonyKey: %v", err)
	}
	statement, err := NewGenesisStatement([]Challenge{
		issueChallenge(t, first, KindGenesis, RoleFounder, publicKey, now),
		issueChallenge(t, second, KindGenesis, RoleFounder, publicKey, now),
	}, publicKey, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewGenesisStatement: %v", err)
	}
	certificate := signWithMembers(t, statement, privateKey, now, first, second)
	groupID, err := StatementID(statement)
	if err != nil {
		t.Fatalf("StatementID(genesis): %v", err)
	}
	return certificate, groupID
}

func makeMembership(t *testing.T, groupID string, target, sponsorA, sponsorB testMember, refs []SponsorRef, now time.Time) Certificate {
	t.Helper()
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatalf("NewCeremonyKey: %v", err)
	}
	statement, err := NewMembershipStatement(
		groupID,
		issueChallenge(t, target, KindMembership, RoleTarget, publicKey, now),
		[]Challenge{
			issueChallenge(t, sponsorA, KindMembership, RoleSponsor, publicKey, now),
			issueChallenge(t, sponsorB, KindMembership, RoleSponsor, publicKey, now),
		},
		refs,
		publicKey,
		now,
		24*time.Hour,
	)
	if err != nil {
		t.Fatalf("NewMembershipStatement: %v", err)
	}
	return signWithMembers(t, statement, privateKey, now, target, sponsorA, sponsorB)
}

func founderRefs(t *testing.T, genesis Certificate, founders ...testMember) []SponsorRef {
	t.Helper()
	certificateID, err := StatementID(genesis.Request.Statement)
	if err != nil {
		t.Fatalf("StatementID(genesis): %v", err)
	}
	refs := make([]SponsorRef, 0, len(founders))
	for _, founder := range founders {
		refs = append(refs, SponsorRef{
			Account:      founder.account,
			CredentialID: CredentialID(certificateID, founder.account),
		})
	}
	return refs
}

func TestVerifierReconstructsGenesisAndMembership(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	third := newTestMember(t, "https://three.example", "user-three")
	verifier := NewVerifier(trustMembers(t, first, second, third))

	genesis, groupID := makeGenesis(t, first, second, testNow)
	membership := makeMembership(t, groupID, third, first, second, founderRefs(t, genesis, first, second), testNow.Add(time.Second))

	group, err := verifier.Reconstruct([]Certificate{membership, genesis}, testNow.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Reconstruct: %v", err)
	}
	if group.ID != groupID {
		t.Fatalf("group id = %q, want %q", group.ID, groupID)
	}
	if got := group.MemberAccounts(); !reflect.DeepEqual(got, []Account{first.account, third.account, second.account}) {
		t.Fatalf("members = %#v", got)
	}
}

func TestCertificateRejectsTampering(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	verifier := NewVerifier(trustMembers(t, first, second))
	genesis, _ := makeGenesis(t, first, second, testNow)

	tests := map[string]func(*Certificate){
		"group": func(c *Certificate) { c.Request.Statement.GroupID = "another-group" },
		"origin": func(c *Certificate) {
			c.Request.Statement.Participants[0].Account.Origin = "https://attacker.example"
		},
		"user":   func(c *Certificate) { c.Request.Statement.Participants[0].Account.UserID = "attacker" },
		"nonce":  func(c *Certificate) { c.Request.Statement.Participants[0].Nonce = "replacement" },
		"expiry": func(c *Certificate) { c.Request.Statement.ExpiresAt++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			tampered := cloneCertificate(genesis)
			mutate(&tampered)
			if _, err := verifier.VerifyCertificate(tampered); !errors.Is(err, ErrInvalidSignature) && !errors.Is(err, ErrInvalidArtifact) {
				t.Fatalf("VerifyCertificate error = %v, want tampering rejection", err)
			}
		})
	}
}

func TestCertificateRequiresEveryOriginKeyToBeTrusted(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	genesis, _ := makeGenesis(t, first, second, testNow)
	verifier := NewVerifier(trustMembers(t, first))

	if _, err := verifier.VerifyCertificate(genesis); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("VerifyCertificate error = %v, want invalid signature for unknown origin key", err)
	}
}

func TestCeremonyCannotBeCompletedWithAnotherPrivateKey(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	publicKey, _, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	_, attackerPrivateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := NewGenesisStatement([]Challenge{
		issueChallenge(t, first, KindGenesis, RoleFounder, publicKey, testNow),
		issueChallenge(t, second, KindGenesis, RoleFounder, publicKey, testNow),
	}, publicKey, testNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignCeremony(statement, attackerPrivateKey); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("SignCeremony error = %v, want mismatched ceremony key", err)
	}
}

func TestApprovalIsIdempotentButChallengeCannotChangeStatements(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := NewGenesisStatement([]Challenge{
		issueChallenge(t, first, KindGenesis, RoleFounder, publicKey, testNow),
		issueChallenge(t, second, KindGenesis, RoleFounder, publicKey, testNow),
	}, publicKey, testNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request, err := SignCeremony(statement, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := first.broker.Approve(first.account, request, testNow)
	if err != nil {
		t.Fatalf("first Approve: %v", err)
	}
	repeated, err := first.broker.Approve(first.account, request, testNow.Add(time.Second))
	if err != nil {
		t.Fatalf("repeated Approve: %v", err)
	}
	if !reflect.DeepEqual(repeated, approval) {
		t.Fatalf("repeated approval differs")
	}

	statement.ExpiresAt++
	changed, err := SignCeremony(statement, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.broker.Approve(first.account, changed, testNow.Add(time.Second)); !errors.Is(err, ErrApprovalAlreadyIssued) {
		t.Fatalf("changed Approve error = %v, want already issued", err)
	}
}

func TestExpiredChallengeIsRejected(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := NewGenesisStatement([]Challenge{
		issueChallenge(t, first, KindGenesis, RoleFounder, publicKey, testNow),
		issueChallenge(t, second, KindGenesis, RoleFounder, publicKey, testNow),
	}, publicKey, testNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request, err := SignCeremony(statement, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.broker.Approve(first.account, request, testNow.Add(DefaultChallengeTTL+time.Second)); !errors.Is(err, ErrChallengeExpired) {
		t.Fatalf("Approve error = %v, want challenge expired", err)
	}
}

func TestOneExistingServerCannotSponsorMembershipTwice(t *testing.T) {
	sponsor := newTestMember(t, "https://sponsor.example", "sponsor-one")
	target := newTestMember(t, "https://target.example", "target")
	secondSponsorAccount := Account{Origin: sponsor.account.Origin, UserID: "sponsor-two"}
	publicKey, _, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	secondChallenge, err := sponsor.broker.IssueChallenge(secondSponsorAccount, KindMembership, RoleSponsor, publicKey, testNow)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewMembershipStatement(
		"group",
		issueChallenge(t, target, KindMembership, RoleTarget, publicKey, testNow),
		[]Challenge{
			issueChallenge(t, sponsor, KindMembership, RoleSponsor, publicKey, testNow),
			secondChallenge,
		},
		[]SponsorRef{
			{Account: sponsor.account, CredentialID: "credential-one"},
			{Account: secondSponsorAccount, CredentialID: "credential-two"},
		},
		publicKey,
		testNow,
		time.Hour,
	)
	if !errors.Is(err, ErrInsufficientSponsors) && !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("NewMembershipStatement error = %v, want insufficient distinct sponsors", err)
	}
}

func TestRevocationRemovesMember(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	third := newTestMember(t, "https://three.example", "user-three")
	verifier := NewVerifier(trustMembers(t, first, second, third))
	genesis, groupID := makeGenesis(t, first, second, testNow)
	membership := makeMembership(t, groupID, third, first, second, founderRefs(t, genesis, first, second), testNow.Add(time.Second))
	membershipID, err := StatementID(membership.Request.Statement)
	if err != nil {
		t.Fatal(err)
	}
	credentialID := CredentialID(membershipID, third.account)
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	revokedAt := testNow.Add(2 * time.Second)
	statement, err := NewRevocationStatement(
		groupID,
		credentialID,
		issueChallenge(t, third, KindRevocation, RoleMember, publicKey, revokedAt),
		publicKey,
		revokedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	revocation := signWithMembers(t, statement, privateKey, revokedAt, third)

	group, err := verifier.Reconstruct([]Certificate{revocation, membership, genesis}, revokedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("Reconstruct: %v", err)
	}
	if got := group.MemberAccounts(); !reflect.DeepEqual(got, []Account{first.account, second.account}) {
		t.Fatalf("members after revocation = %#v", got)
	}
}

func TestMembershipSponsoredAfterRevocationIsRejected(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	third := newTestMember(t, "https://three.example", "user-three")
	verifier := NewVerifier(trustMembers(t, first, second, third))
	genesis, groupID := makeGenesis(t, first, second, testNow)
	genesisID, err := StatementID(genesis.Request.Statement)
	if err != nil {
		t.Fatal(err)
	}
	firstCredentialID := CredentialID(genesisID, first.account)
	revokedAt := testNow.Add(time.Second)
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	revocationStatement, err := NewRevocationStatement(
		groupID,
		firstCredentialID,
		issueChallenge(t, first, KindRevocation, RoleMember, publicKey, revokedAt),
		publicKey,
		revokedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	revocation := signWithMembers(t, revocationStatement, privateKey, revokedAt, first)
	membership := makeMembership(t, groupID, third, first, second, founderRefs(t, genesis, first, second), testNow.Add(2*time.Second))

	if _, err := verifier.Reconstruct([]Certificate{membership, revocation, genesis}, testNow.Add(3*time.Second)); !errors.Is(err, ErrInsufficientSponsors) {
		t.Fatalf("Reconstruct error = %v, want revoked sponsor rejection", err)
	}
}

func TestAccountCannotHoldTwoActiveGroupCredentials(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	target := newTestMember(t, "https://target.example", "target")
	verifier := NewVerifier(trustMembers(t, first, second, target))
	genesis, groupID := makeGenesis(t, first, second, testNow)
	refs := founderRefs(t, genesis, first, second)
	firstMembership := makeMembership(t, groupID, target, first, second, refs, testNow.Add(time.Second))
	secondMembership := makeMembership(t, groupID, target, first, second, refs, testNow.Add(2*time.Second))

	if _, err := verifier.Reconstruct([]Certificate{genesis, firstMembership, secondMembership}, testNow.Add(3*time.Second)); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("Reconstruct error = %v, want duplicate active credential rejection", err)
	}
}

func TestFinalizeIsIdempotentAndResumableAcrossBrokers(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	verifier := NewVerifier(trustMembers(t, first, second))
	genesis, _ := makeGenesis(t, first, second, testNow)

	if err := first.broker.Finalize(genesis, verifier, nil, testNow); err != nil {
		t.Fatalf("first finalize: %v", err)
	}
	if got := len(second.broker.Certificates()); got != 0 {
		t.Fatalf("second certificates before resume = %d", got)
	}
	if err := second.broker.Finalize(genesis, verifier, nil, testNow); err != nil {
		t.Fatalf("resumed second finalize: %v", err)
	}
	if err := second.broker.Finalize(genesis, verifier, nil, testNow); err != nil {
		t.Fatalf("idempotent second finalize: %v", err)
	}
	if got := len(second.broker.Certificates()); got != 1 {
		t.Fatalf("second certificates after resume = %d, want 1", got)
	}
}

func TestTrustStoreRejectsDiscoveryForAnotherFetchOrigin(t *testing.T) {
	honest := newTestMember(t, "https://honest.example", "honest")
	trust := NewTrustStore()
	if err := trust.Add("https://attacker.example", honest.broker.Discovery()); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("TrustStore.Add error = %v, want mismatched-origin rejection", err)
	}
}

func TestChallengeRejectsCeremonyKeySubstitution(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	publicKey, _, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	attackerPublicKey, attackerPrivateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := NewGenesisStatement([]Challenge{
		issueChallenge(t, first, KindGenesis, RoleFounder, publicKey, testNow),
		issueChallenge(t, second, KindGenesis, RoleFounder, publicKey, testNow),
	}, publicKey, testNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	statement.CeremonyPublicKey = attackerPublicKey
	request, err := SignCeremony(statement, attackerPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.broker.Approve(first.account, request, testNow); !errors.Is(err, ErrChallengeMismatch) {
		t.Fatalf("Approve error = %v, want ceremony-key mismatch", err)
	}
}

func TestFreshChallengeRejectsBackdatedStatement(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	challengeTime := testNow.Add(2 * time.Minute)
	statement, err := NewGenesisStatement([]Challenge{
		issueChallenge(t, first, KindGenesis, RoleFounder, publicKey, challengeTime),
		issueChallenge(t, second, KindGenesis, RoleFounder, publicKey, challengeTime),
	}, publicKey, testNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request, err := SignCeremony(statement, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.broker.Approve(first.account, request, challengeTime); !errors.Is(err, ErrChallengeMismatch) {
		t.Fatalf("Approve error = %v, want backdating rejection", err)
	}
}

func TestGroupIDIsDerivedFromUniqueGenesis(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	attackerA := newTestMember(t, "https://attacker-a.example", "attacker-a")
	attackerB := newTestMember(t, "https://attacker-b.example", "attacker-b")
	verifier := NewVerifier(trustMembers(t, first, second, attackerA, attackerB))

	genesis, groupID := makeGenesis(t, first, second, testNow)
	parallelGenesis, parallelGroupID := makeGenesis(t, attackerA, attackerB, testNow)
	if groupID == parallelGroupID {
		t.Fatal("unrelated genesis certificates produced the same group id")
	}
	for _, test := range []struct {
		certificate Certificate
		wantID      string
	}{{genesis, groupID}, {parallelGenesis, parallelGroupID}} {
		group, err := verifier.Reconstruct([]Certificate{test.certificate}, testNow)
		if err != nil {
			t.Fatal(err)
		}
		if group.ID != test.wantID || group.GenesisID != test.wantID {
			t.Fatalf("group identity = (%q, %q), want %q", group.ID, group.GenesisID, test.wantID)
		}
	}
}

func TestCredentialLifetimeIsBounded(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	publicKey, _, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewGenesisStatement([]Challenge{
		issueChallenge(t, first, KindGenesis, RoleFounder, publicKey, testNow),
		issueChallenge(t, second, KindGenesis, RoleFounder, publicKey, testNow),
	}, publicKey, testNow, MaxCredentialTTL+time.Second)
	if !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("NewGenesisStatement error = %v, want excessive-lifetime rejection", err)
	}
}

func TestSponsorApprovalRejectsKnownRevocationBeforeSelectiveFinalization(t *testing.T) {
	first := newTestMember(t, "https://one.example", "user-one")
	second := newTestMember(t, "https://two.example", "user-two")
	target := newTestMember(t, "https://target.example", "target")
	verifier := NewVerifier(trustMembers(t, first, second, target))
	genesis, groupID := makeGenesis(t, first, second, testNow)
	if err := first.broker.Finalize(genesis, verifier, nil, testNow); err != nil {
		t.Fatal(err)
	}

	genesisID, err := StatementID(genesis.Request.Statement)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	revokedAt := testNow.Add(time.Second)
	revocationStatement, err := NewRevocationStatement(
		groupID,
		CredentialID(genesisID, first.account),
		issueChallenge(t, first, KindRevocation, RoleMember, publicKey, revokedAt),
		publicKey,
		revokedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	revocation := signWithMembers(t, revocationStatement, privateKey, revokedAt, first)
	if err := first.broker.Finalize(revocation, verifier, []Certificate{genesis}, revokedAt); err != nil {
		t.Fatal(err)
	}

	publicKey, privateKey, err = NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	membershipAt := revokedAt.Add(time.Second)
	membershipStatement, err := NewMembershipStatement(
		groupID,
		issueChallenge(t, target, KindMembership, RoleTarget, publicKey, membershipAt),
		[]Challenge{
			issueChallenge(t, first, KindMembership, RoleSponsor, publicKey, membershipAt),
			issueChallenge(t, second, KindMembership, RoleSponsor, publicKey, membershipAt),
		},
		founderRefs(t, genesis, first, second),
		publicKey,
		membershipAt,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := SignCeremony(membershipStatement, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.broker.Approve(first.account, request, membershipAt); !errors.Is(err, ErrInsufficientSponsors) {
		t.Fatalf("Approve error = %v, want known-revocation rejection", err)
	}
}
