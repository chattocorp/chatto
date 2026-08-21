# Authling TODO

This file tracks outstanding Authling product decisions and implementation
work. Keep tasks concise and remove them when completed. Record architecture
decisions in `docs/adr/`, implemented feature behavior in `docs/fdr/`, and the
current runtime in `docs/architecture/`.

## First slice: local accounts

- [ ] Extract and harden the KMS, wrapped-key storage, key-cache, and durable-erasure mechanics proven by signup
- [ ] Expand the built-in password blocklist with a maintained compromised-password corpus and update policy

## Product foundations

- [ ] Establish canonical identity, relying-party, client, and account terminology
- [ ] Add standalone diagnostics and backup behavior

## Later account and authentication work

- [ ] Design upstream SSO through Goth-supported providers
- [ ] Define secure upstream-account linking and email-collision behavior
- [ ] Implement an event-backed orphan-key cleanup worker and crash/race tests
- [ ] Implement durable account erasure
- [ ] Implement erasure-aware two-phase replay before destroying account keys
- [ ] Add key-loss, erasure, backup, substitution, and KMS-failure tests
- [ ] Implement upstream SSO and account linking

## OpenID Connect

- [ ] Define authenticated relying-party grouping and exact-client grant migration across one or more OIDC clients
- [ ] Track CIMD Internet-Draft evolution and define compatibility policy before upgrading from draft-02
- [ ] Design signing-key rotation and retirement
- [ ] Add rotating refresh tokens bound to durable authorization-grant generations
- [ ] Add token-revocation and RP-initiated logout behavior
- [ ] Define identity-claim release policies before adding further scopes and claims
- [ ] Automate the official OpenID Provider conformance suite outside the fast Docker-free test path
- [ ] Add version-skew fixtures for CIMD-aware Chatto consumers

## Later user interface work

- [ ] Design MFA-recovery, consent, and account-linking experiences

## Documentation

- [ ] Record accepted Authling architecture decisions as ADRs
- [ ] Add FDRs as Authling features become implemented
- [ ] Keep the Authling glossary and runtime architecture inventory current
- [ ] Document deployment, configuration, administration, and OIDC integration
