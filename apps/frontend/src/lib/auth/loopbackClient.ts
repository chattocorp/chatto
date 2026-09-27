/**
 * OAuth client ID of the built-in loopback client. The bundled frontend uses it
 * on a loopback origin to sign in to a server that is not local, because that
 * server cannot fetch a CIMD document from the user's device. Any local process
 * can present this ID, so the consent page labels it as unverified.
 */
export const LOOPBACK_OAUTH_CLIENT_ID = 'chatto://loopback';
