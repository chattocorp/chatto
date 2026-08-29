# Runtime State Inventory

Key files: [`cli/internal/core/storage.go`](../../cli/internal/core/storage.go),
[`cli/internal/core/read_state_index.go`](../../cli/internal/core/read_state_index.go),
[`cli/internal/core/notification_boundary_index.go`](../../cli/internal/core/notification_boundary_index.go),
[`cli/internal/core/notification_unread_marker.go`](../../cli/internal/core/notification_unread_marker.go),
[`cli/internal/core/notification_materializer.go`](../../cli/internal/core/notification_materializer.go),
[`cli/internal/core/notification_occurrence_model.go`](../../cli/internal/core/notification_occurrence_model.go),
[`cli/internal/core/runtime_token_keys.go`](../../cli/internal/core/runtime_token_keys.go),
[`cli/internal/core/renewable_sessions.go`](../../cli/internal/core/renewable_sessions.go),
[`cli/internal/http_server/browser_session_store.go`](../../cli/internal/http_server/browser_session_store.go),
[`cli/internal/core/external_identities.go`](../../cli/internal/core/external_identities.go),
[`cli/internal/core/asset_uploads.go`](../../cli/internal/core/asset_uploads.go),
[`cli/internal/core/credential_usage.go`](../../cli/internal/core/credential_usage.go), and
[`cli/internal/kms/builtin.go`](../../cli/internal/kms/builtin.go). Protobuf
records live in
[`runtime_state/v1`](../../proto/chatto/core/runtime_state/v1),
[`key_material/v1`](../../proto/chatto/core/key_material/v1), and
[`cache_state/v1`](../../proto/chatto/core/cache_state/v1).

Related decisions: [ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md) and
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md) and
[ADR-081](../adr/ADR-081-explicit-expiry-for-mutable-runtime-credentials.md) and
[ADR-084](../adr/ADR-084-separate-internal-protobufs-by-storage-contract.md).

## KV buckets

| Bucket                        | Storage | Backup   | Description                                     |
| ----------------------------- | ------- | -------- | ----------------------------------------------- |
| `RUNTIME_STATE`               | File    | Yes      | Persisted latest-value runtime/user state, including notification visibility boundaries, credential-usage telemetry, push subscriptions, auth/workflow tokens, wrapped app DEK records, and encrypted snapshot pointers |
| `MEMORY_CACHE`                | Memory  | No       | Volatile cache state: presence, worker leases and cooldowns, reconciliation counters, and worker health heartbeats |
| `ENCRYPTION_KEYS`             | File    | **No**   | KMS KEKs and LiveKit per-call E2EE keys (excluded for security); app-owned wrapped DEKs live in `RUNTIME_STATE` |

**ENCRYPTION_KEYS keys:**

| Key                   | Description                       |
| --------------------- | --------------------------------- |
| `kek.{keyId}`         | Protobuf `UserKeyEncryptionKey` per-user KEK record; the complete object key is also the opaque KMS key ref |
| `user.{userId}`       | Legacy raw 32-byte per-user encryption key retained only for decrypting pre-envelope message bodies; account deletion shreds it with the user's current KEK |
| `call.e2ee.{callId}`  | Protobuf `UserKeyEncryptionKey` record containing the raw LiveKit E2EE key for one active call; referenced by `CallStartedEvent.e2ee_key_ref` and shredded when `CallEndedEvent` commits |

Notes: Excluded from backups so backup archives do not contain the KEKs needed to unwrap protected content, legacy raw user keys, or the per-call media keys needed to decrypt captured LiveKit media. Chatto core uses the in-process [`internal/kms`](../../cli/internal/kms/) boundary for KEK creation, DEK wrap/unwrap, legacy-key lookup, call-key lookup, and key shredding. App-owned wrapped DEK records live in `RUNTIME_STATE` under `dek.{id}`; that complete key is the content-key ref.

The `user.{userId}` namespace remains a live compatibility constraint: message
bodies written before envelope encryption still need that key to remain
readable. It must therefore survive ordinary upgrades and any key-bearing
backup/export and restore until those bodies no longer need to be read. It is
not used for new writes. Crypto-shredding must remove both this legacy key and
the user's current opaque `kek.*` key so deletion covers every message-body
encryption generation.

The backup CLI stages JetStream snapshots in an owner-only random directory
beside the destination and always removes plaintext staging. It publishes
owner-only archives through a same-directory temporary file and atomic rename.

Backup, restore, and key export/import accept passphrases through hidden
terminal prompts or explicit `--passphrase-file` and `--passphrase-stdin`
automation sources. They do not accept passphrases in process arguments.

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
| `read.room.{userId}.{roomId}`          | Last-read root message event ID (UTF-8 string, ~14 bytes). Empty value = "joined but no specific event read yet" (e.g. joined an empty room). Missing key triggers a one-time lazy init to the room's current last event. Membership and DM initialization create the key only when absent. |
| `read.thread.{userId}.{roomId}.{threadRootEventId}` | Latest thread message event ID the user has seen. |
| `notification_read_boundary.{userId}.{roomId}[.{threadRootEventId}]` | Two big-endian EVT stream sequences: the latest room/thread timeline target read and the reaction projection horizon observed by that read action. One process-wide filtered watcher indexes these keys and local writes wait for their exact revision. Every replica performs one startup repair, then reconciles only unread occurrences in the room/thread scope whose boundary changed, completing an interrupted KV-to-`NOTIFICATIONS` handshake idempotently without a periodic global scan. The two coordinates keep reactions that arrive after a read new until the next read. The key expires 90 days after its latest update and account deletion removes it. |
| `notification_visibility_boundary.{userId}.{roomId}` | Big-endian EVT stream sequence of the latest explicit or derived room visibility loss relevant to notification materialization. The same notification-boundary watcher makes fanout eligibility an indexed lookup. Leave/removal request paths record the boundary immediately after commit. The ordered worker records it for those facts and for universal-room, room-group placement, ban, or `room.join`, `message.read`, and `message.read-interactions` RBAC/role changes that remove all applicable access. A partial change removes only output whose exact message target is no longer visible. Delayed source facts at or before a complete-loss boundary cannot reappear after access returns. The key expires after 90 days, matching the maximum lifetime of source facts it can suppress, and account deletion removes it. |
| `notification_unread_marker.{userId}.{roomId}[.{threadRootEventId}]` | Protobuf `NotificationUnreadMarker` for the latest source-time Badge decision in one room or thread scope. It stores the source event, actor, rich signal, and EVT source sequence needed to apply target, reaction, visibility, and read boundaries. A higher source sequence replaces an older value with KV OCC; delayed retries cannot regress it. Thread markers roll up into room Badge attention. The public `has_unread` fields come only from these markers, not from Message Read Cursor lag. The key expires 90 days after its latest source, and account deletion removes it. It is final user-visible output, not prepared notification work. |
| `push_subscription.{userId}.{endpointHash}` | Web Push subscription record (protobuf `PushSubscription`) for a user's browser/device and service-worker registration. The client host identifies the Chatto server that supplied the installed app; the sending server combines it with its own hostname to reconstruct the click route. The endpoint hash keeps multiple devices and per-server scoped subscriptions per user while deduplicating the same browser subscription. A record is deliverable only while its revision matches the endpoint's active owner claim. The browser Push API auth secret and a random per-save cleanup token form a capability for that exact save generation, allowing cancelled work to be removed safely after the account session changes without deleting a later save that reused the browser subscription. |
| `push_endpoint_owner.{sha256(endpoint)}` | JSON Web Push endpoint owner claim containing the active user ID and exact `push_subscription` KV revision. Saves transfer the claim with KV OCC; revision-matched deletes prevent stale logout, expiry cleanup, and subscription rotation races from releasing a newer claim. The renewable push-cleanup lease leader reconciles at startup and every 15 seconds, removing owner claims whose subscription record is missing or no longer matches. Subscription records without a claim remain inert until the browser re-registers. |
| `push_test_notification_throttle.{userId}` | One-byte per-account admission marker owned by [`core/push.go`](../../cli/internal/core/push.go). Atomic creation with a 10-second per-key TTL rate-limits test push attempts across replicas; the marker contains no endpoint or delivery result. |
| `asset_upload.{uploadId}` | JSON room-scoped attachment upload session with actor, declared size/SHA-256, committed offset, chunk keys, status, and expiry. Open sessions use a 15-minute TTL; completed sessions expire with the 24-hour pending-attachment window. |
| `projection_snapshot_pointer.{opaqueLocator}` | Encrypted current/previous generation IDs for one projection and snapshot contract. The opaque locator is derived from both, so different contracts cannot read or overwrite each other. Uses KV revision OCC so stale writers cannot regress newer history within one contract. |
| `credential_usage.bot.{botId}` | Protobuf `CredentialUsageState` with the latest observed use time for each `incoming_webhook:{webhookId}` entry. Successful authentication creates an in-memory observation before request validation or message posting. Each process writes the first observation promptly, coalesces later writes for the same credential to at most one each minute, and uses KV OCC with the maximum timestamp so concurrent writers cannot regress the record. Reads merge a newer local observation. A missing record or entry means that no use was recorded; it does not prove that no use occurred. A read or decode failure produces an explicit unavailable state. Bot response assembly reads telemetry only for resources selected after filtering and pagination. Show-once credential responses skip this read and keep unhydrated fields unspecified. Read or write failure does not affect authentication or message posting. Recording rechecks the projected credential under the same process-local lock that orders revocation cleanup. Each replica also checks projected lifecycle state before and after a write. A periodic sweep removes local and stored telemetry when another replica revoked the credential. A flush that finishes after revocation attempts another delete. The process does not retain revocation tombstones. Bot deletion removes the complete record. The record has no TTL and is included in backups. |
| `email_otp.{hmac(subject)}.{hmac(code)}` | Shared registration and email-verification OTP code JSON. Registration values carry normalized email and, when applicable, the validated invitation ID; authenticated email-verification values carry user ID and email. The subject hash scopes registration by email and authenticated verification by user/email, the code hash verifies the submitted six-digit code, and the raw code is never stored. Uses a configurable per-key TTL that defaults to 30 minutes. |
| `email_otp.{hmac(subject)}.challenge` | Shared OTP challenge JSON with failed-attempt and issued-code counters. Wrong-code attempts update this record revision-safely, five wrong guesses exhaust the challenge until TTL, and at most ten codes can be issued for one challenge window. Uses a configurable per-key TTL that defaults to 30 minutes. |
| `registration_completion.{hmac}` | Registration completion token JSON created after code verification, carrying the invitation ID from an invite-only registration when applicable. Uses per-key 15-minute TTL. |
| `password_reset.{hmac}` | Password reset token JSON. Uses per-key 1-hour TTL and is claimed with a revision-matched delete before the password-change event is appended. |
| `password_reset_request.{hmac(userId)}` | Per-account password-reset delivery reservation containing only the matching HMAC-derived reset-token key. Atomic KV creation permits one prepared link per five-minute window across replicas; failed delivery conditionally deletes the matching reservation before its token so transient cleanup failures remain retryable and do not normally consume the window. Cleanup uses a bounded context detached from request cancellation. |
| `account_deletion_token.{hmac}` | Account deletion confirmation token JSON. Uses per-key 15-minute TTL. |
| `session.{hmac}` | Typed runtime credential JSON with user ID, optional issuing OAuth client ID, credential kind (`first_party_session` or `oauth_access_token`), presentation (`bearer` or `cookie`), source/request metadata, explicit expiry, and the user auth generation it was issued against. Mutable cookie records also contain fresh-auth metadata. They keep one stable SCS handle and an `auth.token_ttl` window (default 90 days). Validation is read-only. In the final quarter, the explicit browser renewal route uses a revision-checked publish to advance the same record's expiry and gives the revision a TTL equal to its remaining explicit lifetime. The response writes the stable handle in a fresh bounded cookie slot so a late response cannot replace a newer browser session. An unrevisioned logout delete fences concurrent renewal. During 0.5 only, the browser migration route can read a 0.4 typed cookie record that has no explicit expiry. It validates and updates the same record with revision OCC, a complete expiry window, and matching physical TTL. Ordinary validation does not accept the old record shape. Immutable bearer access records carry a renewable-session ID and access generation. They use per-key `auth.access_token_ttl` (default 15 minutes), never renew on validation, and are rejected when the stable renewable session is absent or invalid. Bearer validation resolves authoritative fresh-auth values from the stable session. Password and account lifecycle events advance the auth generation; `session.*` scans delete matching records as physical cleanup. OAuth client blocking independently rejects matching delegated records. |
| `renewable_session.{hmac}` | Mutable JSON authority for one human first-party or delegated bearer session: user ID, optional OAuth client ID, kind/source and safe request metadata, creation and current window expiry, auth generation, current refresh generation, last refresh-request verifier and rotation time, and authoritative fresh-auth metadata. The verifier is a purpose-separated HMAC of the raw UUID version 4 recovery nonce. Revision-checked publish serializes refresh rotation across replicas and gives each current revision a per-message TTL equal to its remaining explicit lifetime. A refresh in the final quarter advances the window. Exact retry of the immediately previous generation and request ID recreates the deterministic result during the access lifetime; other stale reuse revokes this key and thereby every access generation. Raw refresh credentials and recovery nonces are never stored. |
| `oauth_authorize.{hmac}` | Validated pending OAuth authorization request containing the exact callback, PKCE challenge, state, client ID, and privacy-safe client display metadata. The signed browser session carries only the opaque raw handle; the HMAC-derived key and JSON value use a 15-minute per-key TTL. Approval, denial, or already-consented completion claims the record with a revision-matched delete. |
| `grant.{hmac}` | OAuth authorization code JSON bound to user ID, optional client ID, exact redirect URI, PKCE challenge, and the user auth generation it was issued against. Uses per-key 5-minute TTL and is claimed with a revision-matched delete before exchange validation and token issuance. |
| `external_identity_create.{hmac}` | Pending account-creation confirmation JSON containing provider identity, optional verified-email/profile hints, and the invitation ID bound by an invite-only browser flow. The KV key is HMAC-derived from the raw capability token, which is never stored; the record uses a 15-minute TTL. |
| `external_identity_link.{hmac}` | Pending link confirmation JSON containing provider identity and optional verified-email/profile hints, bound to the authenticated user. The KV key is HMAC-derived from the raw capability token, which is never stored; the record uses a 15-minute TTL. |
| `external_identity_link_start.{hmac}` | One-time browser handoff JSON containing the provider ID, redirect path, and bound user ID. The KV key is HMAC-derived from the raw capability token, which is never stored; the record uses a 15-minute TTL and is deleted when consumed. |
| `link_preview.{urlHash}` | Versioned cached link preview metadata (protobuf `CachedLinkPreview`) keyed by SHA-256 of the normalized URL. Successful previews use per-key 24-hour TTL; failed fetches use per-key 1-hour TTL. Pre-v1 negative entries refresh once after validated multi-address dialing was added; pre-v1 Mastodon-shaped generic entries also refresh for federated proxy discovery. Current failures and generic fallbacks retain their normal TTL. |
| `link_preview_token.{hmac}` | Short-lived composer link-preview token JSON referencing a cached preview URL. Uses per-key 30-minute TTL; raw tokens are only returned to the client. |
| `dek.{id}` | Wrapped purpose-scoped app DEK record (protobuf `UserDataEncryptionKey`). The complete object key is the content-key ref; it has no TTL and is shredded on account deletion. |

Bot API keys do not create `RUNTIME_STATE` records. Their current HMAC verifier
is a durable user-aggregate fact in `EVT`, projected by `UserAuthProjection`;
this makes key creation and rotation part of the bot's replayable account
history while keeping the raw key show-once. Incoming webhook lifecycle facts
also remain in `EVT`. Only their optional, approximate last-use telemetry is in
`RUNTIME_STATE`.

`ReadStateModel` mirrors both `read.*` key families through one filtered KV
watcher per Chatto process. The initial latest-value watch delivery is a startup
readiness barrier; subsequent local and remote revisions update the same
in-memory index. Request and realtime reconciliation reads use that index
instead of issuing one KV `Get` per room/thread. `RUNTIME_STATE` remains the
authority: writes use KV OCC and wait for their returned revision to reach the
local index when read-your-writes is required. Create-only membership
initialization cannot replace a marker concurrently advanced by the user or
another replica.

Token HMAC keys are derived with `[core].secret_key` and the credential purpose as a domain separator. Backups include `RUNTIME_STATE`, so sessions and pending links survive restore only when the same `core.secret_key` is kept; backup archives do not contain raw bearer access or refresh credentials, cookie credential handles, or raw link/code values. Backups also include wrapped app DEK records, but those records cannot decrypt content without the KEKs in `ENCRYPTION_KEYS` or an external KMS.

**MEMORY_CACHE keys:**

| Key                                        | Description                                      |
| ------------------------------------------ | ------------------------------------------------ |
| `presence.{userId}`                        | Serialized `UserPresence` proto for the user's live status and manual-selection flag; per-key 60s TTL |
| `lease.{name}`                             | Ephemeral coordination record. Current names are `livekit_reconciler`, `projection-snapshot-threads`, `projection-snapshot-expiry`, and `push-subscription-deletion-reconcile`. Snapshot expiry retains a 24-hour cooldown after successful S3 cleanup; push cleanup uses a one-minute cooldown for its bounded late-write pass; the others identify active worker ownership. |
| `livekit.reconciliation.list_failures`      | Shared consecutive LiveKit listing failure counter reset by any successful elected reconciliation pass |

`MEMORY_CACHE` uses memory storage and is neither persisted nor backed up. The
NATS recovery gate recreates the bucket after a full server restart before the
replica returns to readiness.

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

Attachment upload storage: chunked uploads first store temporary `asset-upload.*` chunks in `SERVER_ASSETS`. Completion verifies the full SHA-256, stores the final asset in NATS or S3, records SHA-256/uploader/pending-expiry/video hints in `AssetCreatedEvent`, and deletes temporary chunks. Completed but unattached pending assets expire after 24 hours unless a message attaches them.

### Asset storage and ownership

Asset IDs are globally unique NanoIDs. New public NATS objects use the
`public/` kind segment to make their storage class explicit; private attachments
retain flat keys for compatibility. S3 stores logical, prefix-free keys, with
any configured `path_prefix` applied only at the S3 boundary. Object headers
hold Content-Type and the original filename where available.

S2 compression is enabled for `SERVER_ASSETS`. `MediaModel` owns binary storage
and serving helpers. `AssetModel` owns durable lifecycle facts and shared
durable message-asset deletion recovery.

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

The shared `chatto-asset-cleanup-v1` consumer delivers canonical
`AssetDeletedEvent` facts across replicas. Handlers locate creation metadata by asset ID and
idempotently delete source and derivative bytes plus transform-cache entries.
Beta room-scoped histories without a canonical asset aggregate remain readable
by projections but are not guessed at by the cleanup worker.

Processing events use the same `evt.asset.{assetId}.*` aggregate. The asset
projection also reads beta-era `evt.room.{roomId}.asset_*` facts, allowing 0.1.0
histories to replay without a stream rewrite.

After appending creation and processing-started events, message posting asks
the durable asset-processing queue to start video or animated-GIF processing.
There is no transient NATS Core worker subject or `video_processed` live
signal. Boot recovery derives missed work from EVT projections and calls the
same local path.

Successful processing records a thumbnail plus either historical/animated-GIF
MP4 variant IDs or an HLS manifest containing rendition metadata and ordered
segment IDs with durations. Only segment binaries are durable HLS derivatives;
HTTP handlers generate playlists from the manifest. Each derivative binary is
separately declared with `AssetCreatedEvent`, a role, and an owner pointing to
the original asset. Histories created before HLS remain MP4-only and are not
backfilled. `AssetProcessingFailedEvent.failure_code` records failed or
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
