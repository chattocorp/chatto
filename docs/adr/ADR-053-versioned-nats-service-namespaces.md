# ADR-053: Versioned NATS Service Namespaces

**Date:** 2026-07-20

## Context

Chatto runtime units already share one NATS account and can run either inside
`chatto run` or as separate processes. Search is the first runtime unit that
needs a request/reply contract: the main app exposes an authenticated public
API, while a replaceable provider answers searches over NATS. Future work is
also expected to expose selected Chatto Core operations to trusted server-side
integrations through NATS.

Without a namespace and versioning convention, service subjects would grow
ad hoc. Core-owned operations and replaceable extension points would be hard to
distinguish in logs and NATS permissions, and incompatible payload changes
would have no clean coexistence path.

## Decision

Reserve `svc.>` for versioned Chatto request/reply services. Subjects use this
grammar:

`svc.{servingAuthority}.{service}.{majorVersion}.{endpoint}`

The initial serving-authority roots are:

- `svc.chatto.>` for services implemented by Chatto Core; and
- `svc.chatto_ext.>` for extension contracts implemented by replaceable
  providers, including implementations bundled with Chatto.

For example, a future Core room lookup may use
`svc.chatto.rooms.v1.get`, while the message-search extension uses
`svc.chatto_ext.search.v1.query`. The distinction describes which side serves
the request, not who authored or distributed the implementation.

Each service contract uses protobuf request and response messages. Its subject
major version corresponds to the protobuf contract's major wire version, so an
incompatible replacement can use a new subject while old and new providers
coexist during deployment. Compatible fields evolve additively within one
major version. Subject tokens and protobuf packages are integration contracts
and are not renamed as implementation refactors.

Services use the NATS micro service framework for standard discovery,
monitoring, queue groups, and `Nats-Service-Error` response headers. Domain
endpoints may define a protobuf error-details payload, but callers must handle
the standard error code and description without requiring those details.
Successful request and response bodies contain only the protobuf payload.

Multiple equivalent replicas of one provider may queue-subscribe to an
endpoint. Operators must not run semantically different provider
implementations in the same queue group: NATS may route any request to any
member. A provider must not advertise readiness until it can satisfy the
endpoint's consistency contract.

NATS services are trusted server-side integration surfaces, not normal client
APIs. NATS account permissions restrict which subjects a process may publish
or subscribe to. Extension providers can subscribe narrowly under their
`svc.chatto_ext.{service}.>` contract and receive publish access only to the
Core services they need. Internal stream positions or storage identities may
cross a trusted NATS service when its contract requires them, but public HTTP,
ConnectRPC, and realtime APIs must still seal or replace those coordinates.

One Chatto deployment continues to own one NATS account namespace, so service
subjects do not carry a server or tenant ID. Sharing one account between
independent Chatto deployments would require a separate namespace decision.

## Consequences

Core services and extension-provided services are easy to distinguish in
subject permissions, diagnostics, and operational tooling. Bundled providers
exercise the same extension contract as separately deployed providers, so a
monolithic process does not create a private shortcut around the integration
boundary.

Major versions can coexist during rolling deployments. Additive protobuf
changes remain the preferred evolution within a version, while a breaking
contract receives a new subject and protobuf version.

The service boundary does not make an extension untrusted. A search provider
that consumes encrypted event history and builds a content index necessarily
has privileged access to message content and key material. Operators must
grant NATS permissions accordingly and treat such providers as part of the
trusted server deployment.

Core operations exposed over NATS still go through their owning model or
service boundaries. A NATS endpoint is a transport adapter, not permission to
bypass event publishing, OCC, projection readiness, or domain authorization.
