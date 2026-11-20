# ADR-125: Store User Data in the User's Own AT Protocol PDS

**Status:** Proposed
**Date:** 2026-11-19

## Context

Some data belongs to the user and not to a Chatto server. The server
catalogue is the main example: the servers that the user added, their order,
and the known servers of the Server Directory. UI preferences are another
example. Today the frontend keeps this data in device storage
([ADR-107](ADR-107-keep-chat-data-out-of-device-storage.md)). A user who adds
a server on one device does not see it on a different device.

This data must stay independent of all Chatto servers. A server that stores
the catalogue learns all other servers of its users. A modified server can
read or change that list.

We examined these alternatives:

- **Authling account data.** Authling had an experimental data space. Authling
  ADR-007 removed it: Authling is an identity provider only. A personal-data
  product needs its own decision and trust boundary.
- **Public AT Protocol repository records.** Repository records are public.
  The records then show which servers a user belongs to.
- **Encrypted public records.** The content stays private, but the number and
  time of records are public. The client also needs a key that all devices of
  the user share, which is a new key-management problem.
- **A PDS that Chatto hosts.** It makes Chatto responsible for the storage of
  personal data before users need it. Users who already have a PDS gain
  nothing from it.

AT Protocol spaces store non-public data with explicit access control. A space
can have one member, for settings and similar data. The spaces alpha started
in August 2026. Its protocol design, SDKs, and storage schema are not final.

The frontend that a Chatto server serves is the code of that server. A
modified server can change it. If that frontend receives access to the user's
PDS, the server can read the user data
([ADR-124](ADR-124-atproto-handle-verification-needs-an-issuer.md) describes
the same limit for verified handles).

## Decision

### 1. Device storage is the default

The device-stored catalogue and preferences stay the complete default
behavior. Chatto is fully usable without AT Protocol. A server operator does
not configure, run, or contact AT Protocol services for this feature.

### 2. Synchronized user data lives in a space on the user's own PDS

A user can turn on synchronization. The client then keeps the user data in a
single-member AT Protocol space on the user's own PDS. Any PDS that supports
spaces is permitted. Chatto does not host a PDS for this feature.

The user can turn synchronization off again. The device keeps its local copy.
The client offers to delete the records from the PDS.

### 3. Only frontends with an AT Protocol client identity can synchronize

The client must have its own AT Protocol OAuth client identity. A frontend
without this configuration does not show the feature and does not run its
code.

- **Official clients:** the standalone frontend at `https://app.chatto.run`,
  Chatto Desktop, and Chatto mobile.
- **Standalone frontend image:** the entrypoint writes the AT Protocol client
  metadata only when its configuration enables it, in the same way as the
  Chatto OAuth metadata for `CHATTO_FRONTEND_ORIGIN`. The default is off. An
  operator who enables it for a different origin is the trusted party for
  that origin.
- **Server-served frontend:** the frontend that a Chatto server embeds never
  synchronizes user data, also when the server has an `atproto` sign-in
  provider.

A member of a self-hosted server who wants synchronization opens that server
in an official client
([ADR-025](ADR-025-multi-instance-client-architecture.md)).

### 4. Chatto servers never receive the data or the PDS session

The client connects to the PDS directly. Chatto servers do not receive the
PDS tokens, the user's DID for this purpose, or the synchronized records.
Server sign-in with an AT Protocol identity
([FDR-058](../fdr/FDR-058-atproto-sign-in.md)) stays a separate flow with its
own consent for each server.

### 5. Credentials stay on the device

Synchronized user data never contains Chatto sessions, bearer tokens, refresh
tokens, or other credentials. The PDS operator can read the space. After a
device receives a server from the synchronized catalogue, the user signs in
to that server on that device.

### 6. AT Protocol is an implementation detail

- The frontend reads and writes user data through a storage adapter. The
  device store and the AT Protocol space are two implementations. The
  storage adapter of ADR-112 phase 2 is the seam for the server catalogue.
- AT Protocol code stays in the AT Protocol adapter. Catalogue and
  preference code does not use AT Protocol types or terms.
- The UI uses the term "Internet Handle"
  ([glossary](../GLOSSARY.md)). It does not present Chatto as a Bluesky or
  AT Protocol app.
- The records use the `run.chatto.*` lexicon namespace. The lexicons are a
  public contract. [ADR-045](ADR-045-public-api-stability-tiers.md)
  applies to them.

### 7. Implementation waits for stable spaces

We do not ship AT Protocol synchronization while spaces are an alpha. We can
build the storage adapter with only the device implementation before then.

## Consequences

- Users with a PDS get the same servers and preferences on all official
  clients. Other users see no change.
- Self-hosted operators have no new configuration or dependency.
- Chatto does not store personal data for this feature. The PDS operator that
  the user selected stores it.
- The trust in the synchronized data depends on the Chatto project, which
  serves `app.chatto.run` and ships Desktop and mobile.
- FDR-058 decision 3 ("a remembered handle instead of a device session")
  changes for official clients: they can hold an AT Protocol session of their
  own. The FDR must be updated with the implementation.
- The feature depends on PDS support for spaces. A PDS without spaces support
  cannot synchronize. The client then uses device storage and tells the user
  why.
- The client contacts the user's handle domain, the PLC directory for
  `did:plc` identities, and the user's PDS and its authorization server. The
  client tells the user about these connections before synchronization starts.
  Logs do not contain handles, DIDs, or tokens.
- The records stay readable to the PDS operator. They are private to the
  space, but not end-to-end encrypted.

## Later Phases

These need their own decisions:

- **A Chatto-hosted PDS** as an easy way for new users to get an AT Protocol
  account. It is an additional option, not a requirement.
- **Authling as the identity service for that PDS:** DIDs, handles such as
  `alice.chatto.id`, and the authorization server in front of the PDS. This
  stays within Authling's identity-provider scope (Authling ADR-007). Authling
  does not store the user data.

## Open Questions

- **Client identities:** an AT Protocol client metadata document declares one
  application type. Do the web and native clients need separate client IDs?
- **Records:** which preferences synchronize, in addition to the server
  catalogue? Device-specific values, such as pane widths, probably do not.
- **Conflicts:** last write wins for each record is probably sufficient for
  the catalogue. The first merge of an existing device catalogue needs a rule.
- **Server sign-in:** can an official client with an AT Protocol session offer
  **Continue as @handle** on each server without the handle page?

## Related

- [ADR-025: Multi-Server Client Architecture](ADR-025-multi-instance-client-architecture.md)
- [ADR-045: Public API Stability Tiers](ADR-045-public-api-stability-tiers.md)
- [ADR-107: Keep Chat Data out of Device Storage](ADR-107-keep-chat-data-out-of-device-storage.md)
- [ADR-112: Keep the Server Catalogue in the Frontend](ADR-112-keep-the-server-catalogue-in-the-frontend.md)
- [ADR-124: Verified AT Protocol Handles Need an Independent Issuer](ADR-124-atproto-handle-verification-needs-an-issuer.md)
- [FDR-042: Chatto Neighbors](../fdr/FDR-042-chatto-neighbors.md)
- [FDR-058: AT Protocol Sign-In](../fdr/FDR-058-atproto-sign-in.md)
- Authling ADR-007: Limit Authling to an Identity Provider
