---
name: "chatto-client-placement"
description: "Decide whether Chatto client code goes into `@chatto/client` (packages/chatto-client) or the bundled frontend (apps/frontend). Use before you add a request, store, state, helper, or protocol rule to either, or when you move code between them."
---

# Placement of Chatto Client Code

`@chatto/client` is the one Chatto client (ADR-111). The bundled frontend,
ChattoBot, and third-party bots and frontends use it. The frontend uses the
client. The client never calls into the frontend.

## Decision Procedure

Ask these questions in sequence. Stop at the first "yes".

1. **Does it show something to a person?** Translated text, toasts, sounds,
   routes, components, layout, or Svelte context: put it in the frontend.
2. **Does it talk to a server or keep server data?** Requests, sessions and
   tokens, realtime delivery, server and room data, the operations on this
   data, or a privacy or authorization boundary: put it in the client.
3. **Does it encode a protocol fact?** A token format in a message body,
   mention rules, the permission structure, or validation that mirrors the
   server: put it in the client, also when only the frontend uses it now. The
   frontend keeps only the display part, such as labels and formatting.
4. **Does only one screen or flow need the state?** Search sessions,
   highlights, editor drafts, call media, scroll positions, sidebar shape, or
   device UI preferences: put it in the frontend, in `serverUi` or
   `$lib/state`.
5. **Not sure?** Ask: "Does a bot or a different frontend need this to use
   Chatto correctly?" If yes, put it in the client.

## Placement Table

| Concern                        | `@chatto/client`                                                         | Frontend                                                          |
| ------------------------------ | ------------------------------------------------------------------------ | ----------------------------------------------------------------- |
| A new RPC                      | Facade in `src/api/`                                                     | Only the allowed modules below                                    |
| Server and room data           | Stores in `src/server/` and `src/room/`                                  | Derive from the store in components                               |
| A copy of server data          | Emit a boundary event (`src/server/storeEvents.ts`) for new boundaries   | Keep the copy and clear it at the boundary events                 |
| Per-server UI state            | None                                                                     | `serverUi(store)` in `$lib/state/server/serverUi.ts`              |
| Snapshot query caches          | None                                                                     | `$lib/query`; `cacheRegistry.ts` connects them to boundary events |
| Errors                         | Keep the error object                                                    | `errorMessage()` or `toastError()` from `$lib/utils/errorMessage` |
| Render data from client stores | `src/timeline/`                                                          | Other render data in `$lib/render`                                |
| Text in client data            | Take labels from the host (`DirectMessageLabels` in `timeline/users.ts`) | Supply translated labels (`$lib/render/directMessageLabels.ts`)   |
| Server catalogue policy        | Registry API only (ADR-112)                                              | `$lib/serverCatalogue`                                            |

## Features With Parts In Both

Most features have three parts:

1. A facade in the client's `src/api/`.
2. State in a client server or room store, when hosts read the data.
3. Frontend UI state in `serverUi`. It derives from the store. When it keeps
   a copy of server data, it clears the copy at the store's boundary events:
   `onReset`, `onRoomAccessLost`, `onRoomAccessRestored`, `onUserDeleted`,
   `onAuthorityChanged`, `onPermissionsChanged`, `onSessionEnded`, and
   `onDispose`.

Use these features as examples:

- **Message search.** Facade: `packages/chatto-client/src/api/messageSearch.ts`.
  The client keeps no search state. The frontend keeps the search sessions in
  `$lib/state/server/messageSearch.ts`. `serverUi.ts` clears them at
  `onReset`, `onRoomAccessLost`, `onUserDeleted`, and `onPermissionsChanged`.
- **Voice calls.** Facade: `src/api/voiceCalls.ts`. Store state:
  `projection.activeCalls`. The frontend keeps the LiveKit media, sounds, and
  toasts in `$lib/state/server/voiceCall.svelte.ts`.
- **Notifications.** Facade: `src/api/notifications.ts`. Store:
  `src/server/notifications.ts`, with `notificationTarget`. The frontend
  keeps attention and read views (`notificationAttention.ts`, `readViews.ts`)
  and turns targets into routes (`$lib/notificationPath.ts`).

## Allowed Frontend API Modules

The frontend calls ConnectRPC services directly only for its own screens:
admin tools, first-run setup, Web Push (`$lib/api`), and the cross-tab
session channel (`$lib/auth/sessionChannel.ts`). Do not add other requests
there.

## Known Misplacements

These frontend modules contain protocol facts that belong in the client. Do
not use them as patterns. When you change one, move its protocol part into
the client and keep the display part in the frontend.

- `$lib/messageTimestamps.ts`: the `<t:EPOCH:F>` token format.
- `$lib/customStatusTemplates.ts`: the `chatto:status:<id>` format.
- `$lib/mentions.ts`: mention detection that agrees with the server.
- `$lib/permissions.ts`: the permission structure (scopes, `privileged`,
  `includes`) that mirrors `cli/internal/core/permission.go`. The translated
  descriptions stay in the frontend.
- `$lib/state/server/presenceTracking.ts`: the presence refresh loop and the
  request sequence. The stored presence choice stays a device preference.
- `$lib/api/externalIdentities.ts`: account requests. They are not on the
  allowed list.
- `$lib/state/server/roomUnread.ts` and `roomDirectory.ts`: optimistic
  membership and read-state overlays. These are a gray zone. Another
  frontend must make them again.

## Common Mistakes

- A request helper in `$lib` because "only the frontend uses it".
- A client store that imports host state or calls back into the UI.
- Display text, translated text, or toasts in the client.
- A frontend copy of server data that does not clear at boundary events.
- Svelte runes, `svelte/reactivity`, or `$lib` imports in the client outside
  `src/svelte/`.

## Related Rules

- [`packages/chatto-client/AGENTS.md`](../../../packages/chatto-client/AGENTS.md):
  how to write client code.
- [`apps/frontend/AGENTS.md`](../../../apps/frontend/AGENTS.md): how to write
  frontend code.
- [ADR-111](../../../docs/adr/ADR-111-move-client-state-into-chatto-client.md)
  and [ADR-112](../../../docs/adr/ADR-112-keep-the-server-catalogue-in-the-frontend.md):
  the decisions.
