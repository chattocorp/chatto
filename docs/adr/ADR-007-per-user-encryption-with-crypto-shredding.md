# ADR-007: Per-User Encryption Keys with Crypto-Shredding for GDPR

**Date:** 2026-03-01

## Context

GDPR's "right to erasure" requires that a user's data can be effectively deleted on request. In a chat application, a user's messages are spread across many streams and rooms. Finding and deleting every message is slow, error-prone, and may leave fragments in backups or replicas.

An alternative is **crypto-shredding**: encrypt each user's messages with a key unique to them, and delete the key when erasure is requested. The encrypted data becomes unreadable without the key, achieving the same practical effect as deletion.

## Decision

Use per-user encryption with crypto-shredding:

- **Algorithm**: New message bodies use a compact versioned envelope: XChaCha20-Poly1305 encrypts the body with the author's active message-body DEK epoch. Each DEK is generated as a durable `UserDEKGeneratedEvent` and wrapped by the KMS boundary using the event's opaque wrapping key reference. Legacy bodies encrypted directly with the per-user ChaCha20-Poly1305 key remain readable.
- **AAD binding**: New envelopes authenticate the message event context (event ID, room ID, author ID, message-body DEK epoch, and message-body event type) as Additional Authenticated Data so ciphertext cannot be replayed into a different message context without detection.
- **Durable user PII encryption**: Durable user events encrypt login, display name, and verified email fields with the user's active user-PII DEK epoch. Message bodies and user PII use separate purpose-scoped DEKs, each with its own per-purpose epoch counter. Legacy KV user records are imported by emitting encrypted user events; if an older boot already wrote plaintext user EVT facts, the migration appends encrypted repair facts from the legacy KV source.
- **Per-user keys**: Each user has their own KEK. New KEKs are stored behind opaque KMS key refs in the dedicated `ENCRYPTION_KEYS` KV bucket; DEK events record the key ref used to wrap them.
- **Key isolation**: The encryption key bucket is explicitly excluded from `chatto backup`. Backups contain only encrypted data, never the keys to read it.
- **Erasure = key deletion + durable shred event**: When a user requests deletion, the KMS key refs recorded on their DEK events are shredded and a `UserKeyShreddedEvent` is appended to the user aggregate. All their encrypted message bodies and durable PII payloads become permanently unreadable, and projections treat the shred event as the authoritative tombstone signal before attempting decrypts.
- **Message-owned assets are deleted explicitly**: Attachments and derivative assets are not encrypted with the user's message key. Account deletion therefore records `AssetDeletedEvent`s for message-owned asset graphs and removes their backing bytes separately from crypto-shredding. User avatar assets follow the same durable delete-event path during account deletion.
- **KMS boundary**: DEK operations (`createKey`, `wrapContentKey`, `unwrapContentKey`, `shredKey`) go through a dedicated `internal/kms` interface keyed by opaque KMS refs rather than Chatto user IDs. The default implementation is in-process and backed by `ENCRYPTION_KEYS`; it can be extracted to a standalone service for high-security deployments. Legacy direct-key body decrypt is the only remaining raw-KEK compatibility path.

## Consequences

- **Fast, reliable erasure**: Shredding the user's KMS key refs renders their encrypted message bodies and durable PII unreadable, including v2 bodies whose DEKs are wrapped by those keys. The durable shred event lets room timeline and thread projections immediately tombstone already-projected or replayed messages authored by that user.
- **Backup safety**: Since keys are excluded from backups, restoring a backup does not restore the ability to read deleted users' messages.
- **Attachment cleanup is separate from crypto-shredding**: Binary assets need explicit delete events and storage cleanup because key deletion alone does not affect stored bytes or signed asset locators. Projections stop resolving deleted assets before backing bytes are removed.
- **No content indexing**: Encrypted message bodies cannot be indexed for full-text search on the server. Search features must either work on metadata or require client-side decryption.
- **Key loss is permanent**: If the KMS loses a user's key (outside of intentional deletion), their messages are gone. The KV bucket must be treated as critical data.
- **Per-message overhead**: Legacy bodies carry one nonce and Poly1305 tag. V2 message bodies carry a body nonce, Poly1305 tag, and compact `content_key_epoch` payload field referencing the author's message-body DEK. Wrapped DEK material is stored once per user, purpose, and epoch in the user EVT stream.
- **Durable PII is crypto-shreddable**: New login, display name, and verified-email event payloads become unrecoverable after the user's KEK is destroyed. Cold projection replay skips encrypted PII that can no longer be decrypted.
- **Future extensibility**: The KMS interface, wrapping key refs, and wrapping metadata on DEK events can be adapted to external key management (HashiCorp Vault, AWS KMS, HSM) without changing application code.
