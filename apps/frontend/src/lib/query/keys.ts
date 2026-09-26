import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

/**
 * Root of every cached query that belongs to one server.
 *
 * `removeServerQueries` and `refreshServerQueries` match this prefix. A server
 * read keyed outside it survives sign-out and other privacy boundaries.
 */
export function serverQueryRoot(serverId: string) {
  return ['server', serverId] as const;
}

/**
 * Root for a server read made with one session's credentials.
 *
 * Key every authenticated server read under this root. The session segment
 * keeps a later session from reading an earlier session's responses, and the
 * fixed layout lets cache predicates match `key[2] === 'session'` and the
 * feature segment at `key[4]`.
 */
export function serverSessionQueryRoot(
  serverId: string,
  connection: Pick<ServerConnection, 'queryScope'>
) {
  return [...serverQueryRoot(serverId), 'session', connection.queryScope] as const;
}
