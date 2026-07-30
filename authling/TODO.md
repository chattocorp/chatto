# Authling TODO

This file tracks outstanding Authling product decisions and implementation
work. Keep tasks concise and remove them when completed. Record architecture
decisions in `docs/adr/`, implemented feature behavior in `docs/fdr/`, and the
current runtime in `docs/architecture/`.

## Product foundations

- [ ] Update `AGENTS.md` with the confirmed product direction and engineering boundaries
- [ ] Define Authling's initial product milestone and supported deployment model
- [ ] Establish canonical identity, application, client, account, and document terminology
- [ ] Integrate Authling with the shared event-sourcing module
- [ ] Extract the application-neutral encryption and key-management mechanics Authling needs
- [ ] Define Authling-owned NATS and JetStream resources without copying Chatto policy
- [ ] Design standalone configuration, lifecycle, diagnostics, and backup behavior

## Accounts and authentication

- [ ] Design email-and-password signup, verification, login, and password recovery
- [ ] Define password storage, credential rotation, session, and revocation policies
- [ ] Design upstream SSO through Goth-supported providers
- [ ] Define secure upstream-account linking and email-collision behavior
- [ ] Implement hierarchical user and data keys with encrypted event payloads
- [ ] Implement durable account erasure and orphan-key reconciliation
- [ ] Add key-loss, erasure, backup, substitution, and KMS-failure tests
- [ ] Implement the account and authentication event model
- [ ] Implement email-and-password authentication
- [ ] Implement upstream SSO and account linking

## OpenID Connect

- [ ] Define Authling's initial OIDC profile and security requirements
- [ ] Decide whether an application and an OIDC client are the same resource
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

## User interface

- [ ] Choose between a Go-native frontend and an extracted reusable Svelte SPA package
- [ ] Design signup, login, recovery, consent, and account-linking experiences
- [ ] Implement the initial user-facing authentication flows

## Documentation

- [ ] Record accepted Authling architecture decisions as ADRs
- [ ] Add FDRs as Authling features become implemented
- [ ] Keep the Authling glossary and runtime architecture inventory current
- [ ] Document deployment, configuration, administration, and OIDC integration
