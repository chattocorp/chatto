# Authling TODO

This file tracks outstanding Authling product decisions and implementation
work. Keep tasks concise and remove them when completed. Record architecture
decisions in `docs/adr/`, implemented feature behavior in `docs/fdr/`, and the
current runtime in `docs/architecture/`.

## First slice: local accounts

- [ ] Extract and harden the KMS, wrapped-key storage, key-cache, and durable-erasure mechanics proven by signup
- [ ] Expand the built-in password blocklist with a maintained compromised-password corpus and update policy

## Product foundations

- [ ] Establish canonical identity, application, client, account, and document terminology
- [ ] Add standalone diagnostics and backup behavior

## Later account and authentication work

- [ ] Add password recovery and email-address change flows
- [ ] Define credential rotation and session revocation policies
- [ ] Design upstream SSO through Goth-supported providers
- [ ] Define secure upstream-account linking and email-collision behavior
- [ ] Implement an event-backed orphan-key cleanup worker and crash/race tests
- [ ] Implement durable account erasure
- [ ] Implement erasure-aware two-phase replay before destroying account keys
- [ ] Add key-loss, erasure, backup, substitution, and KMS-failure tests
- [ ] Implement upstream SSO and account linking

## OpenID Connect

- [ ] Define applications as consent and document boundaries containing one or more OIDC clients
- [ ] Define ownership and authorization for attaching additional clients to an application
- [ ] Model Chatto server backends and browser frontends as distinct OIDC clients
- [ ] Track CIMD Internet-Draft evolution and define compatibility policy before upgrading from draft-02
- [ ] Design signing-key rotation and retirement
- [ ] Add refresh-token, token-revocation, and RP-initiated logout behavior
- [ ] Add additional scopes and claims only with an explicit data-release policy
- [ ] Automate the official OpenID Provider conformance suite outside the fast Docker-free test path
- [ ] Add version-skew fixtures for CIMD-aware Chatto consumers

## User documents

- [ ] Define the per-user, per-application document ownership and authorization model
- [ ] Decide whether untyped documents contain JSON, arbitrary bytes, or both
- [ ] Decide the independently erasable key granularity for user documents
- [ ] Define key validation, enumeration, concurrency, deletion, size, and quota semantics
- [ ] Design the authenticated document API
- [ ] Implement app-scoped document storage
- [ ] Add isolation, concurrency, quota, and data-deletion tests

## Later user interface work

- [ ] Design recovery, consent, and account-linking experiences

## Documentation

- [ ] Record accepted Authling architecture decisions as ADRs
- [ ] Add FDRs as Authling features become implemented
- [ ] Keep the Authling glossary and runtime architecture inventory current
- [ ] Document deployment, configuration, administration, and OIDC integration
