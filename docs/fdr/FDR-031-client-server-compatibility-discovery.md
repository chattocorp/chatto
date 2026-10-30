# FDR-031: Client–Server Compatibility Discovery

**Status:** Experimental
**Last reviewed:** 2026-10-30

## Overview

The multi-server client compares each registered Chatto server's software
version with the oldest release that the client supports, shows the server's
current version, and warns when the client and server cannot provide the
expected experience. This gives people useful upgrade guidance while
Chatto's pre-1.0 API remains experimental.

## Behavior

- A registered server's Server information page starts with its public profile:
  logo, name, banner, description, and welcome message. It uses the same branding
  component and Markdown rendering as the login page and omits empty optional
  fields. Hostname and software version follow in Server details.
- A registered server's context menu or touch sheet shows its configured host
  and provides an action that copies that exact host, including a non-default
  port, to the clipboard. The information page also has this copy action.
- The menu groups Server information, Copy server hostname, and Mark as read
  together. Ordering controls form a second group, shown only when more than
  one server is registered. Retry, sign-in/out, and removal form the final group.
  The menu header shows the name and hostname; the version is on the information page.
- Remote servers have two version thresholds. Full feature support requires
  `0.5.0-beta.18`. The minimum connectable release is `0.5.0-beta.9`.
  Between these thresholds, the client attempts the normal UI and realtime
  connection. The server menu shows a short summary and a Server information
  action. The Compatibility section of that page lists known feature limitations,
  the release required for each feature, and one overall upgrade recommendation.
  Affected views show a short link to the relevant section of that page.
  The page also shows supported, unknown, and unreachable states. Remote-server
  reports remain accessible when chat cannot connect or the user is signed out.
  Thread views require beta.16 to separate followed threads from threads
  started by the viewer. Older servers can show thread results, with a notice
  that the lists may not be separated correctly. Independent channel and
  thread Slow Mode requires beta.18. Older servers keep a shared cooldown
  and a single setting; room field presence still controls the runtime fallback. Below the
  connection threshold, it blocks chat routes and the realtime connection.
- A connected server's version stays current without a reload. The client
  reads the server profile again at each full realtime catch-up. A full
  catch-up occurs when the live connection reconnects, for example after a
  server upgrade, and at each poll of an inactive server. A limited-version
  warning clears when the updated version meets the full-support threshold.
- The origin server serves its own frontend and has no version check.
- For a remote server, Server information also lists product features that
  need a newer bundled frontend. Both directions use one Affected features
  list, with the same row layout and one combined count. The upgrade notice
  identifies which side needs an update. Each entry shows the server's feature name,
  explanation, and the Chatto release that introduced support. One notice
  shows the current app version and asks the user to update the app or ask
  the administrator of the server that provides it. This advice never blocks
  a connection. Permissions and server settings still control feature access.
  A newer server version alone does not produce this notice.
- The server supplies an advisory feature catalog through the public
  `ServerDiscoveryService.ListFeatures` endpoint.
  Older frontends can display new feature IDs without an update. Names and
  descriptions are plain text, rendered without HTML or Markdown. The endpoint
  accepts an optional locale. The server tries that locale and its parents,
  then English for missing translations. Invalid or unsupported locales also
  use English. The optional `introduced_after_version` filter returns
  only features introduced after that Chatto release. It requires
  a full Semantic Version; invalid values return `INVALID_ARGUMENT`. Omit the
  filter for the complete catalog. Third-party clients should omit it.
  The app loads the catalog only on Server information for remote servers.
  It sends its selected language and normalized frontend release. Responses
  are cached in memory by server, server version, language, and version filter.
  A language or server version change selects a new cache entry. The app does
  not request the catalog for its origin server. `GetServer` and
  `/.well-known/chatto` do not include the catalog.
  Older servers that return `UNIMPLEMENTED` produce no frontend upgrade advice.
  Other catalog errors show a retry action without blocking chat. Unknown
  frontend or feature versions do not establish a limitation. Unreachable
  servers do not show retained frontend upgrade advice.
- Custom suffixes do not affect comparison. For example,
  `0.6.1-my-customization` compares as `0.6.1`, and
  `0.6.1-beta.2-my-customization` compares as `0.6.1-beta.2`. Alpha, beta,
  and rc stages retain their order. Build metadata is ignored. The UI shows
  the original version string. Unparseable versions remain unknown; the
  client attempts a connection and shows a warning.
- An unreachable server remains registered and is reported as unreachable
  rather than being assigned a healthy or compatible state.
- A server icon in the gutter has two states: normal and warning. The warning
  appears for a connection problem or compatibility warning: a required
  sign-in, an unreachable server, an unsupported or unknown server version, or
  a failed connection. The icon is not dimmed.
- Only the result of an attempt changes the state. A failed attempt shows the
  warning, and only a successful attempt removes it. A new attempt, an
  immediate retry after a dropped connection, or a credential renewal does not
  change the state. Tab wake and network recovery start again from the normal
  state, because results from before the wake are out of date.
- The server context menu and touch sheet explain the problem. When more than
  one problem applies, they show the first problem in the order above.
- When discovery finds a remote version below the connection threshold, or cannot
  reach the server, the server route shows an explanation screen instead of
  the server sidebar and its pages. The screen names the server and its host,
  explains the problem, shows the server version and the required version
  when they apply, and has a Check Again action. Check Again runs discovery
  again. When the result allows a connection, the server opens normally.
  The screen stays visible while a new attempt runs. For a remote server, the
  screen also offers **Remove server**, with the same confirmation as the
  gutter menu. The origin server cannot be removed.
- The Server Directory allows users to add limited and unknown versions when
  sign-in is available. Versions below the connection threshold still link to
  their own client.
- Third-party clients own and test their own minimum supported server release.
- The `chatto.realtime.v1` protobuf namespace uses behavioral protocol version
  4 for the public event stream. The alpha server rejects older and unknown
  handshakes.

## Design Decisions

### 1. Separate full support from the connection threshold

**Decision:** The client records a minimum connectable release and a map of
minimum releases for individual features. Full support is derived from the
highest requirement, including the connection minimum. Known limitations
appear in Server information, linked from the server menu and affected views.
This keeps persistent notices short as the requirement map grows. Feature support
is separate from server configuration and viewer permissions. Unknown versions
do not produce a list of confirmed limitations. The thread-view notice is
advisory and does not block requests.
Increase the connection threshold only as
a last resort, including during alpha and beta releases. First consider a
compatible API design, use of the existing server contract, or deferring the
new dependency. If an increase is necessary, record the exact incompatibility,
why reasonable alternatives are insufficient, and the required upgrade action.
Use the oldest server release that meets the requirement.
**Why:** A new optional feature must not block otherwise usable chat. The
connection threshold keeps the previous full-support baseline, beta.9. The
beta.16 increase added separate followed and own thread views, while the
authentication and realtime handshake contracts remained unchanged. Earlier
releases need separate verification before the connection threshold can be
lowered. See changes #2710 and #2983.
**Tradeoff:** Some operations can fail between the two thresholds. Unknown
custom builds can also fail at runtime. The client keeps normal authentication,
permission, operation-error, and realtime protocol-rejection handling.

### 2. The client owns compatibility policy

**Decision:** Unauthenticated discovery reports the server software version.
Each client owns its server version thresholds and compares that policy
with the discovered version before connecting.
**Why:** Future clients know which older server contracts they still implement;
the server cannot predict the requirements of clients that do not exist yet.
The advisory feature catalog is separate from connection policy. It records
the first Chatto release for each listed product feature, not a
minimum client version required to connect. Third-party clients use stable
feature IDs with their own support policy.
**Tradeoff:** A client update must keep its version thresholds accurate and
cannot rely on a server to reject it on the client's behalf.
The catalog must be maintained when a feature ships. It is not a complete
API capability list. Each feature needs English fallback text and translations.
This first version shows frontend upgrade advice
on Server information only; it does not add a server-gutter warning.

### 3. Registration data does not cache compatibility conclusions

**Decision:** The client keeps version and compatibility results in live
per-server state and refreshes them from discovery instead of persisting them
with the registered server and its credentials.
**Why:** Persisted compatibility information would become stale across server
and client upgrades. The registry should retain connection identity, while the
server state owns current discovery facts.
Full realtime catch-up refreshes the version of a connected server, because an
upgrade restarts the server without an event that reports the new version.
**Tradeoff:** Compatibility is unknown until discovery completes after the
client starts. Each full catch-up makes one more small request.

### 4. Pre-1.0 compatibility remains advisory

**Decision:** Compatibility discovery informs connection decisions and warnings but
does not turn the experimental `v1` packages into a stability guarantee.
**Why:** Chatto still needs room to reshape its public API in response to early
feedback. ADR-045 requires intentional review and migration guidance for
breaks without prematurely freezing the API.
**Tradeoff:** Integrators must still pin server versions and read release notes.

## Related

- **ADRs:** ADR-025 (multi-instance client architecture), ADR-042 (protobuf-first public API), ADR-045 (public API stability tiers), ADR-067 (Electron desktop packaging), ADR-091 (semantic realtime events)
- **FDRs:** FDR-017 (Room Groups & Sidebar Layout), FDR-023 (Authentication & Sessions), FDR-027 (PWA & Service Worker), FDR-034 (Chatto Desktop), FDR-045 (Realtime Event Stream)
