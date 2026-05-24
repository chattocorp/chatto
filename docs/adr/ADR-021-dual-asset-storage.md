# ADR-021: Dual Asset Storage — NATS ObjectStore Default, S3 Optional

**Date:** 2026-03-01

## Context

Chatto stores binary assets: user avatars, space icons, and message attachments. The storage backend must work out of the box for small self-hosted instances but scale for larger deployments with terabytes of files.

The options are:

- **S3-only**: Simple, scalable, well-understood. But requires operators to provision an S3-compatible service, breaking the zero-dependency self-hosted goal.
- **NATS ObjectStore only**: Uses the existing NATS infrastructure, no external dependencies. But NATS isn't optimized for large blob storage and operators have no escape hatch.
- **Local filesystem**: Fast, simple. But not portable across processes in a clustered deployment.
- **Default to NATS, optional S3**: Zero dependencies for small instances, scalable storage for large ones.

## Decision

Support two storage backends with NATS ObjectStore as the default:

- **NATS ObjectStore** (default): Assets stored in JetStream-backed object store buckets. Works out of the box with zero configuration.
- **S3-compatible storage** (optional): When configured, new uploads go to S3. Existing NATS-stored assets continue to be served from NATS.

Each `Attachment` proto carries a `Storage` field — a oneof selecting either a `NATSAsset{key}` or an `S3Asset{key, bucket}` — populated at upload time. Retrieval reads `Storage` off the proto and goes directly to the indicated backend. The backend choice is per-attachment, not per-deployment, so a host that switches from NATS to S3 can serve both eras side by side without any migration step.

## Consequences

- **Zero-dependency default**: Small instances run entirely on embedded NATS. No S3 bucket to provision, no IAM credentials to configure.
- **Gradual migration path**: Operators can enable S3 at any time. New uploads go to S3; old assets remain in NATS. No downtime, no bulk migration required.
- **No backend probing on retrieval**: Each `Attachment` records its backend on `Storage` at upload time. The HTTP handler reads it directly and goes to the right place, no NATS-then-S3 fallthrough. Switching backends doesn't trigger any extra lookups.
- **Deletion follows `Storage`**: `DeleteAttachmentFromStorage(*Attachment)` branches on the Storage type and deletes from exactly one backend. The proto knows where its bytes live.
- **NATS storage limits matter for NATS-only deployments**: Large instances using NATS-only storage will eventually hit practical limits (disk space, stream size). The S3 option is the escape hatch for this.
- **Pre-ADR-030-Phase-4 S3 keys live at `spaces/{server|DM}/attachments/{id}`** instead of the current `attachments/{id}` layout. Their full key is preserved on `Attachment.Storage.S3.Key`, so we read them at exactly that path — no kind-segment probing needed.
