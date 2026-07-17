# Runtime State Inventory

Key files: [`cli/internal/core/core.go`](../../cli/internal/core/core.go),
[`cli/internal/core/runtime_token_keys.go`](../../cli/internal/core/runtime_token_keys.go),
[`cli/internal/core/external_identities.go`](../../cli/internal/core/external_identities.go),
[`cli/internal/core/asset_uploads.go`](../../cli/internal/core/asset_uploads.go), and
[`cli/internal/projectionsnapshot/repository.go`](../../cli/internal/projectionsnapshot/repository.go)

Related decision: [ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md).

## KV buckets

| Bucket                        | Storage | Backup   | Description                                     |
| ----------------------------- | ------- | -------- | ----------------------------------------------- |
| `RUNTIME_STATE`               | File    | Yes      | Persisted latest-value runtime/user state, including pending notifications, push subscriptions, auth/workflow tokens, wrapped app DEK records, and encrypted snapshot pointers |
| `MEMORY_CACHE`                | Memory  | No       | Volatile cache state: presence, worker leases and cooldowns, reconciliation counters, and worker health heartbeats |
| `ENCRYPTION_KEYS`             | File    | **No**   | KMS KEKs and LiveKit per-call E2EE keys (excluded for security); app-owned wrapped DEKs live in `RUNTIME_STATE` |

**ENCRYPTION_KEYS keys:**

| Key                   | Description                       |
| --------------------- | --------------------------------- |
| `kek.{keyId}`         | Protobuf `UserKeyEncryptionKey` per-user KEK record; the complete object key is also the opaque KMS key ref |
| `call.e2ee.{callId}`  | Protobuf `UserKeyEncryptionKey` record containing the raw LiveKit E2EE key for one active call; referenced by `CallStartedEvent.e2ee_key_ref` and shredded when `CallEndedEvent` commits |

Notes: Excluded from backups so backup archives do not contain the KEKs needed to unwrap protected content or the per-call media keys needed to decrypt captured LiveKit media. Chatto core uses the in-process [`internal/kms`](../../cli/internal/kms/) boundary for KEK creation, DEK wrap/unwrap, call-key lookup, and key shredding. App-owned wrapped DEK records live in `RUNTIME_STATE` under `dek.{id}`; that complete key is the content-key ref.

The backup CLI stages JetStream snapshots in an owner-only random directory
beside the destination and always removes plaintext staging. It publishes
owner-only archives through a same-directory temporary file and atomic rename.

Backup, restore, and key export/import accept passphrases through hidden
terminal prompts or explicit `--passphrase-file` and `--passphrase-stdin`
automation sources. The process-argument `--passphrase` compatibility path is
deprecated for removal in 0.5.

Restore extracts into an owner-only temporary directory. Before connecting
restored paths to JetStream, it rejects non-local manifest stream names and
unsupported tar entry types, and bounds archive entry count, individual file
size, and total expanded bytes.

**RUNTIME\_STATE keys:**

`RUNTIME_STATE` is the persisted home for latest-value runtime state that
survives restart but is not content/domain history. See
[ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md).

| Key                                    | Description                                                       |
| -------------------------------------- | ----------------------------------------------------------------- |
| `read.room.{userId}.{roomId}`          | Last-read root message event ID (UTF-8 string, ~14 bytes). Empty value = "joined but no specific event read yet" (e.g. joined an empty room). Missing key triggers a one-time lazy init to the room's current last event. |
| `read.thread.{userId}.{roomId}.{threadRootEventId}` | Latest thread message event ID the user has seen. |
| `notification.{userId}.{notificationId}` | Pending notification record (protobuf `Notification`) for DM messages, @mentions, replies, and all-message subscriptions. Uses per-key 90-day TTL. Live sync uses `NotificationCreatedEvent` / `NotificationDismissedEvent` on `live.sync.user.{userId}.*`; DND keeps the record but marks creation sync silent and skips push delivery. |
| `push_subscription.{userId}.{endpointHash}` | Web Push subscription record (protobuf `PushSubscription`) for a user's browser/device. The endpoint hash keeps multiple devices per user while deduplicating the same browser subscription. A record is deliverable only while its revision matches the endpoint's active owner claim. |
| `push_endpoint_owner.{sha256(endpoint)}` | JSON Web Push endpoint owner claim containing the active user ID and exact `push_subscription` KV revision. Saves transfer the claim with KV OCC; revision-matched deletes prevent stale logout, expiry cleanup, and subscription rotation races from releasing a newer claim. Legacy subscription records without a claim remain inert until the browser re-registers. |
| `asset_upload.{uploadId}` | JSON room-scoped attachment upload session with actor, declared size/SHA-256, committed offset, chunk keys, status, and expiry. Open sessions use a 15-minute TTL; completed sessions expire with the 24-hour pending-attachment claim window. |
| `projection_snapshot_pointer.{opaqueLocator}` | Encrypted current/previous generation IDs for one projection and snapshot contract. The opaque locator is derived from both, so different contracts cannot read or overwrite each other. Uses KV revision OCC so stale writers cannot regress newer history within one contract. |
| `email_otp.{hmac(subject)}.{hmac(code)}` | Shared registration and email-verification OTP code JSON. Registration values carry normalized email; authenticated email-verification values carry user ID and email. The subject hash scopes registration by email and authenticated verification by user/email, the code hash verifies the submitted six-digit code, and the raw code is never stored. Uses per-key 15-minute TTL. |
| `email_otp.{hmac(subject)}.challenge` | Shared OTP challenge JSON with failed-attempt and issued-code counters. Wrong-code attempts update this record revision-safely, five wrong guesses exhaust the challenge until TTL, and at most ten codes can be issued for one challenge window. Uses per-key 15-minute TTL. |
| `registration_completion.{hmac}` | Registration completion token JSON created after code verification. Uses per-key 15-minute TTL. |
| `password_reset.{hmac}` | Password reset token JSON. Uses per-key 1-hour TTL and is claimed with a revision-matched delete before the password-change event is appended. |
| `password_reset_request.{hmac(userId)}` | Per-account password-reset delivery reservation containing only the matching HMAC-derived reset-token key. Atomic KV creation permits one prepared link per five-minute window across replicas; failed delivery conditionally deletes the matching reservation before its token so transient cleanup failures remain retryable and do not normally consume the window. Cleanup uses a bounded context detached from request cancellation. |
| `account_deletion_token.{hmac}` | Account deletion confirmation token JSON. Uses per-key 15-minute TTL. |
| `session.{hmac}` | Typed runtime credential JSON with user ID, credential kind (`first_party_session` or `oauth_access_token`), presentation (`bearer` or `cookie`), source/request metadata, fresh-auth metadata, and the user auth generation it was issued against. Uses per-key `auth.token_ttl` (default 90 days); successful validation refreshes the key with a new per-key TTL for sliding-window expiry. Password resets, password changes, external identity disconnects, and account deletion revoke older credentials by advancing the user's auth generation through durable user events; scans of `session.*` delete matching records as cleanup. |
| `cookie_session.{userId}.{sessionHmac}` | Deprecated legacy cookie-session protobuf (`CookieSession`) retained for validation and cleanup of sessions created before typed runtime credentials. Current login flows write cookie-presentation credentials to `session.{hmac}` instead. Remove this compatibility path after existing sessions age out or after a documented pre-1.0 cutoff. |
| `grant.{hmac}` | OAuth authorization code JSON with the user auth generation it was issued against. Uses per-key 5-minute TTL and is claimed with a revision-matched delete before exchange validation and token issuance. |
| `external_identity_create.{hmac}` | Pending account-creation confirmation JSON containing provider identity and optional verified-email/profile hints. The KV key is HMAC-derived from the raw capability token, which is never stored; the record uses a 15-minute TTL. |
| `external_identity_link.{hmac}` | Pending link confirmation JSON containing provider identity and optional verified-email/profile hints, bound to the authenticated user. The KV key is HMAC-derived from the raw capability token, which is never stored; the record uses a 15-minute TTL. |
| `external_identity_link_start.{hmac}` | One-time browser handoff JSON containing the provider ID, redirect path, and bound user ID. The KV key is HMAC-derived from the raw capability token, which is never stored; the record uses a 15-minute TTL and is deleted when consumed. |
| `link_preview.{urlHash}` | Versioned cached link preview metadata (protobuf `CachedLinkPreview`) keyed by SHA-256 of the normalized URL. Successful previews use per-key 24-hour TTL; failed fetches use per-key 1-hour TTL. Pre-v1 negative entries refresh once after validated multi-address dialing was added; pre-v1 Mastodon-shaped generic entries also refresh for federated proxy discovery. Current failures and generic fallbacks retain their normal TTL. |
| `link_preview_token.{hmac}` | Short-lived composer link-preview token JSON referencing a cached preview URL. Uses per-key 30-minute TTL; raw tokens are only returned to the client. |
| `dek.{id}` | Wrapped purpose-scoped app DEK record (protobuf `UserDataEncryptionKey`). The complete object key is the content-key ref; it has no TTL and is shredded on account deletion. |

Token HMAC keys are derived with `[core].secret_key` and the token family as a domain separator. Backups include `RUNTIME_STATE`, so sessions and pending links survive restore only when the same `core.secret_key` is kept; backup archives do not contain raw bearer tokens, cookie credential handles, or raw link/code values. Backups also include wrapped app DEK records, but those records cannot decrypt content without the KEKs in `ENCRYPTION_KEYS` or an external KMS.

**MEMORY_CACHE keys:**

| Key                                        | Description                                      |
| ------------------------------------------ | ------------------------------------------------ |
| `presence.{userId}`                        | Serialized `UserPresence` proto for the user's live status and manual-selection flag; per-key 60s TTL |
| `lease.{name}`                             | Ephemeral coordination record. Current names are `livekit_reconciler`, `asset_cleanup`, `projection-snapshot-threads`, and `projection-snapshot-expiry`. The expiry record is retained as a 24-hour cooldown after successful S3 cleanup; the others identify active worker ownership. |
| `livekit.reconciliation.list_failures`      | Shared consecutive LiveKit listing failure counter reset by any successful elected reconciliation pass |
| `asset_cleanup.status`                     | Privacy-safe JSON heartbeat from the elected physical asset-deletion worker. Records worker ownership, initial-scan/pass state, pending retry count and age, last pass/success times, and the last inspected EVT sequence. |

`MEMORY_CACHE` uses memory storage and is neither persisted nor backed up.

Presence uses per-key TTL with a 30-second client refresh and `LimitMarkerTTL`,
so NATS emits delete markers on expiry. A single per-process **PresenceHub**
watches `presence.>`, retains the current snapshot for bulk API response
hydration, and emits `PresenceChanged` only when a user's status changes.
Singular mutation responses still read KV directly when they require
read-your-writes. Clients refresh through `MyAccountService.UpdatePresence`;
disconnect and "look offline" stop refreshing instead of writing `OFFLINE`.

Ephemeral `lease.{name}` records coordinate singleton background work and
periodic cooldowns across replicas without adding durable state. Active voice
call participants come from the call-state projection over durable room EVT
facts and are reconciled against LiveKit by the elected reconciler. Per-call
LiveKit E2EE keys remain behind the KMS boundary in `ENCRYPTION_KEYS`; the retired `CALL_STATE` bucket is
no longer imported.

## Object Store buckets

| Bucket                      | Description                                       |
| --------------------------- | ------------------------------------------------- |
| `ASSET_CACHE`               | Cached resized images (optional)                  |
| `PROJECTION_SNAPSHOTS`      | Encrypted projection snapshots with configurable TTL (optional) |
| `SERVER_ASSETS`             | NATS-backed persisted asset binaries              |

**ASSET_CACHE keys:**

| Key                                                  | Description                                      |
| ---------------------------------------------------- | ------------------------------------------------ |
| `attachment-stable-v2.{attachmentId}.{paramsHash}`   | Cached attachment derivative at specific bounds |
| `server.{assetId}.{paramsHash}`                      | Cached transform of a server asset               |

Notes: Only created when `[core.assets.cache]` is enabled in config. Uses TTL for automatic expiration (default 7 days). Current cache entries for deleted assets are also evicted from the active attachment or server prefix during binary cleanup. Attachment cache namespaces are versioned when encoding changes so older bytes are not reused. `paramsHash` is first 16 hex chars of SHA256(`{width}x{height}_{fit}`). S2 compression enabled.

**PROJECTION_SNAPSHOTS keys:**

| Key | Description |
| --- | --- |
| `internal/projection-snapshots/{projection}/{contract}/objects/{opaqueEpoch}/{generationId}` | Encrypted and compressed snapshot generation. Contract IDs are projection-scoped, the secret-derived epoch isolates generations across secret changes, and random generation IDs are referenced by the encrypted `RUNTIME_STATE` pointer. The same logical key shape is used by NATS and S3. |

**SERVER\_ASSETS keys:**

| Key         | Description                                    |
| ----------- | ---------------------------------------------- |
| `public/{assetId}` | New public avatars, server branding, and link-preview images |
| `{assetId}` | Private attachment binaries and historical flat-key public assets |
| `asset-upload.{uploadId}.{offset:020}.{chunkId}` | Temporary attachment upload chunk before completion; the zero-padded offset supports ordering and the unique chunk ID prevents replacement races |

**S3 asset keys:**

| Key                     | Description                                                  |
| ----------------------- | ------------------------------------------------------------ |
| `attachments/{assetId}` | Message attachment originals and derivative binaries         |
| `instance/{assetId}`    | Server-scoped assets: user avatars, server branding images, and link-preview images |

Attachment upload storage: chunked uploads first store temporary `asset-upload.*` chunks in `SERVER_ASSETS`. Completion verifies the full SHA-256, stores the final attachment in NATS or S3, records SHA-256/uploader/pending-expiry/video hints in `AssetCreatedEvent`, and deletes temporary chunks. Completed but unclaimed pending attachment assets expire after 24 hours unless a message body claims them.

### Asset storage and ownership

Asset IDs are globally unique NanoIDs. New public NATS objects use the
`public/` kind segment to make their storage class explicit; private attachments
retain flat keys for compatibility. S3 stores logical, prefix-free keys, with
any configured `path_prefix` applied only at the S3 boundary. Object headers
hold Content-Type and the original filename where available.

S2 compression is enabled for `SERVER_ASSETS`. `MediaModel` owns binary storage
and serving helpers. `AssetModel` owns durable lifecycle facts and elected
message-asset deletion recovery.

Asset metadata is created in `AssetCreatedEvent` on
`evt.asset.{assetId}.asset_created`. Room scope and ownership context live on
the event as `message`, `derivative`, `user_avatar`, or `server_branding`, not
inside `Asset`. New message bodies reference message-owned assets by ID.

Link preview images are server-scoped persisted assets embedded in message
bodies as `AssetRecord` values. Generic previews use `LinkPreview.image_asset`;
structured social-post snapshots can also carry an author avatar, website-card
image, up to four post images, and one quoted post with the same media fields.
Provider adapters populate the same bounded snapshot shape, and all media in
the outer and quoted posts shares one fetch budget. Each record identifies whether its image lives in S3 or
`SERVER_ASSETS`; `image_asset_id` remains for older generic previews.

### Asset lifecycle and compatibility

The `asset_cleanup` lease holder incrementally replays canonical
`AssetDeletedEvent` facts. It locates creation metadata by asset ID and
idempotently deletes source and derivative bytes plus transform-cache entries.
Beta room-scoped histories without a canonical asset aggregate remain readable
by projections but are not guessed at by the cleanup worker.

Processing events use the same `evt.asset.{assetId}.*` aggregate. The asset
projection also reads beta-era `evt.room.{roomId}.asset_*` facts, allowing 0.1.0
histories to replay without a stream rewrite.

After appending creation and processing-started events, message posting asks
the process-local video service to start video or animated-GIF processing.
There is no transient NATS Core worker subject or `video_processed` live
signal. Boot recovery derives missed work from EVT projections and calls the
same local path.

Successful processing records thumbnail and variant asset IDs. Each derivative
binary is separately declared with `AssetCreatedEvent` and an owner pointing to
the original asset. `AssetProcessingFailedEvent.failure_code` records failed or
unavailable outcomes.

Account deletion follows the projected message asset graph. It appends
`AssetDeletedEvent` for source assets and derivative children before deleting
their backing bytes.

`/assets/server/*` is an unauthenticated route limited to positively classified
public server assets. New NATS-backed URLs use
`/assets/server/public/{assetId}` and map directly to the explicit
`public/{assetId}` object namespace. Canonical `/assets/server/{assetId}` URLs
remain aliases and preserve historical flat-key URLs.

Before transform signature parsing, resize-cache access, object reads, or image
transformation, the handler rejects unknown namespaces, every live or deleted
`AssetProjection` declaration, and private NATS metadata (`Room-Id` or
`Upload-Id`). Historical avatars and branding are recognized through current
durable pointers. Historical link-preview images are recognized through
durable message-body references.

S3 public delivery probes only `instance/{assetId}`. This route never probes
private current or historical attachment prefixes. Disallowed classes return
404.
