import type { ServerConnection } from '@chatto/client/server/serverConnection';

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
 * The server and session that a query key belongs to.
 *
 * A `ServerScope` and a `SessionSnapshot` have this shape, so components pass
 * them directly. Code without a scope passes `{ serverId, connection }`.
 */
export type QueryKeyScope = {
  readonly serverId: string;
  readonly connection: Pick<ServerConnection, 'queryScope'>;
};

/**
 * Root for a server read made with one session's credentials.
 *
 * Key every authenticated server read under this root. The session segment
 * keeps a later session from reading an earlier session's responses, and the
 * fixed layout lets cache predicates match `key[2] === 'session'` and the
 * feature segment at `key[4]`.
 */
export function serverSessionQueryRoot(scope: QueryKeyScope) {
  return [...serverQueryRoot(scope.serverId), 'session', scope.connection.queryScope] as const;
}

/** The roles, account age, and verified identities cached for one profile and viewer session. */
export function profileDetailsKey(scope: QueryKeyScope, userId: string) {
  return [...serverSessionQueryRoot(scope), 'user', userId, 'profile-details'] as const;
}
