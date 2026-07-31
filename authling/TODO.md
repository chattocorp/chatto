# Authling TODO

This file tracks outstanding Authling product decisions and implementation
work. Keep tasks concise and remove them when completed. Record architecture
decisions in `docs/adr/`, implemented feature behavior in `docs/fdr/`, and the
current runtime in `docs/architecture/`.

## First slice: local accounts

- [ ] Build the initial server-rendered templ signup and login pages
- [ ] Define the initial email-and-password account creation, login, and logout behavior
- [ ] Extract the KMS, wrapped-key storage, key-cache, and durable-erasure mechanics needed by the first slice
- [ ] Implement hierarchical user and data keys with encrypted account and credential payloads
- [ ] Implement email-and-password account creation
- [ ] Implement login, session handling, and logout
- [ ] Add end-to-end and adversarial tests for the complete local-account flow

## Product foundations

- [ ] Establish canonical identity, application, client, account, and document terminology
- [ ] Add standalone diagnostics and backup behavior

## Later account and authentication work

- [ ] Add email verification and password recovery
- [ ] Define credential rotation and session revocation policies
- [ ] Design upstream SSO through Goth-supported providers
- [ ] Define secure upstream-account linking and email-collision behavior
- [ ] Implement durable account erasure and orphan-key reconciliation
- [ ] Add key-loss, erasure, backup, substitution, and KMS-failure tests
- [ ] Implement upstream SSO and account linking

## OpenID Connect

- [ ] Record one immutable issuer per Authling deployment and account IDs as public `sub` values
- [ ] Define Authling's initial OIDC profile and security requirements
- [ ] Define applications as consent and document boundaries containing one or more OIDC clients
- [ ] Define ownership and authorization for attaching additional clients to an application
- [ ] Model Chatto server backends and browser frontends as distinct OIDC clients
- [ ] Design automatic standards-based client onboarding without manual preregistration
- [ ] Adopt Client ID Metadata Documents as the primary automatic onboarding mechanism and track draft evolution
- [ ] Design client registration, redirect URI, scope, claim, and consent behavior
- [ ] Design signing-key storage, publication, rotation, and retirement
- [ ] Implement discovery metadata and the JWKS endpoint
- [ ] Implement Authorization Code flow with PKCE
- [ ] Implement token issuance, refresh, revocation, and user information
- [ ] Add OIDC conformance, version-skew, and adversarial security tests

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
