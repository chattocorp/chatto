/**
 * AT Protocol account links (FDR-056, ADR-123).
 *
 * A Chatto server publishes a user's DID as an unverified claim. The user's
 * own client proves the claim with an account link record in the user's AT
 * Protocol repository. Only the DID's owner can write that record, so a
 * modified server cannot forge it. Every viewer's client verifies the record
 * itself and shows the handle only after a successful verification.
 *
 * Verification contacts these external systems from the viewer's device:
 * the PLC directory (for `did:plc`), the `did:web` host, a DNS-over-HTTPS
 * resolver and the handle's domain (for the handle), and the profile owner's
 * PDS. Call it only when a profile is shown.
 */

import { LRUCache } from 'lru-cache';

/** Collection of Chatto account link records. */
export const ATPROTO_ACCOUNT_LINK_COLLECTION = 'run.chatto.accountLink';

/** Prefix that separates account link record keys from other hashes. */
const RECORD_KEY_DOMAIN = 'chatto-account-link:v1';

const PLC_DIRECTORY_URL = 'https://plc.directory';
/** DNS-over-HTTPS resolver for `_atproto` TXT records. Browsers have no DNS API. */
const DNS_OVER_HTTPS_URL = 'https://cloudflare-dns.com/dns-query';
const REQUEST_TIMEOUT_MS = 10_000;
const CACHE_TTL_MS = 10 * 60 * 1000;

/** `fetch` signature that verification and record writes use. */
export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

/**
 * Authenticated access to one AT Protocol repository, for example an OAuth
 * session of `@atproto/oauth-client-browser`. `fetchHandler` resolves
 * `pathname` against the user's PDS and adds the credentials.
 */
export type AtprotoRepoSession = {
  did: string;
  fetchHandler(pathname: string, init: RequestInit): Promise<Response>;
};

/** Identity of one Chatto account that a link record refers to. */
export type ChattoAccountRef = {
  /**
   * Origin through which the client reaches the server, for example
   * `https://chat.example.com`. This is normally the server's configured
   * public URL. A client that uses a different host name cannot verify links
   * that were written for the public URL.
   */
  serverOrigin: string;
  /** Chatto user ID on that server. */
  userId: string;
};

/** Result of verifying a published DID. */
export type AtprotoVerification =
  | { status: 'verified'; did: string; handle: string }
  | {
      status: 'unverified';
      did: string;
      /** Why verification failed. `unavailable` can succeed on a later try. */
      reason: 'invalid_did' | 'no_handle' | 'handle_mismatch' | 'no_link' | 'unavailable';
    };

/** Parsed parts of a DID document that AT Protocol uses. */
export type AtprotoIdentity = {
  did: string;
  /** Handle that the DID document claims; `null` without a claim. */
  handle: string | null;
  /** PDS base URL; `null` without a PDS service. */
  pds: string | null;
};

class UnavailableError extends Error {}

/**
 * Record key of the link record for `account`. It is a SHA-256 hash, so the
 * public repository does not show which servers the user is on.
 */
export async function accountLinkRecordKey(account: ChattoAccountRef): Promise<string> {
  const origin = canonicalOrigin(account.serverOrigin);
  const input = new TextEncoder().encode(`${RECORD_KEY_DOMAIN}\n${origin}\n${account.userId}`);
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', input));
  return Array.from(digest, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

/** Lexicon method that Chatto sign-in service tokens name. */
export const ATPROTO_SIGN_IN_METHOD = 'run.chatto.signIn';

/**
 * Audience of service tokens for the Chatto server at `serverOrigin`: its
 * did:web identity with the `#chatto` service. Returns `null` when the server
 * cannot accept AT Protocol sign-in, such as an HTTPS origin with a port.
 *
 * Derive the audience from the origin that the client connects to, never
 * from data that the server supplies. A server that could choose the audience
 * could request a token for another server and use it there. The server
 * derives the same value from its configured public URL.
 */
export function atprotoServiceAudience(serverOrigin: string): string | null {
  let url: URL;
  try {
    url = new URL(serverOrigin);
  } catch {
    return null;
  }
  const host = url.hostname.toLowerCase();
  if (url.protocol === 'https:') {
    return url.port ? null : `did:web:${host}#chatto`;
  }
  if (url.protocol !== 'http:' || !isLocalDevelopmentHost(host)) return null;
  // AT Protocol allows a port in did:web only for localhost.
  return url.port ? `did:web:localhost%3A${url.port}#chatto` : 'did:web:localhost#chatto';
}

function isLocalDevelopmentHost(host: string): boolean {
  return (
    host === 'localhost' || host.endsWith('.localhost') || host === '127.0.0.1' || host === '[::1]'
  );
}

/** Write the link record for `account` into the session's repository. */
export async function writeAccountLinkRecord(
  session: AtprotoRepoSession,
  account: ChattoAccountRef
): Promise<void> {
  const response = await session.fetchHandler('/xrpc/com.atproto.repo.putRecord', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      repo: session.did,
      collection: ATPROTO_ACCOUNT_LINK_COLLECTION,
      rkey: await accountLinkRecordKey(account),
      record: { $type: ATPROTO_ACCOUNT_LINK_COLLECTION, createdAt: new Date().toISOString() }
    })
  });
  if (!response.ok)
    throw new Error(`The account link could not be saved (HTTP ${response.status}).`);
}

/** Delete the link record for `account`. A missing record is not an error. */
export async function deleteAccountLinkRecord(
  session: AtprotoRepoSession,
  account: ChattoAccountRef
): Promise<void> {
  const response = await session.fetchHandler('/xrpc/com.atproto.repo.deleteRecord', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      repo: session.did,
      collection: ATPROTO_ACCOUNT_LINK_COLLECTION,
      rkey: await accountLinkRecordKey(account)
    })
  });
  if (!response.ok) {
    throw new Error(`The account link could not be removed (HTTP ${response.status}).`);
  }
}

/** Verifies published DIDs and caches the results of one client. */
export type AtprotoLinkVerifier = {
  /**
   * Verify that `did` controls the Chatto `account`. Results are cached for
   * ten minutes; results with reason `unavailable` are not cached.
   */
  verify(did: string, account: ChattoAccountRef): Promise<AtprotoVerification>;
  /** Drop a cached result, for example after the user changed the link. */
  forget(did: string, account: ChattoAccountRef): void;
};

/** Create a verifier. `fetchImpl` defaults to the global `fetch`. */
export function createAtprotoLinkVerifier(
  fetchImpl: FetchLike = (input, init) => globalThis.fetch(input, init)
): AtprotoLinkVerifier {
  const cache = new LRUCache<string, Promise<AtprotoVerification>>({ max: 500, ttl: CACHE_TTL_MS });
  const cacheKey = (did: string, account: ChattoAccountRef) =>
    `${did}\n${canonicalOrigin(account.serverOrigin)}\n${account.userId}`;
  return {
    verify(did, account) {
      const key = cacheKey(did, account);
      const cached = cache.get(key);
      if (cached) return cached;
      const pending = verifyUncached(did, account, fetchImpl).then((result) => {
        if (result.status === 'unverified' && result.reason === 'unavailable') cache.delete(key);
        return result;
      });
      cache.set(key, pending);
      // A rejected promise must not stay cached.
      pending.catch(() => cache.delete(key));
      return pending;
    },
    forget(did, account) {
      cache.delete(cacheKey(did, account));
    }
  };
}

async function verifyUncached(
  did: string,
  account: ChattoAccountRef,
  fetchImpl: FetchLike
): Promise<AtprotoVerification> {
  try {
    if (!isAtprotoDid(did)) return { status: 'unverified', did, reason: 'invalid_did' };
    const identity = await resolveAtprotoIdentity(did, fetchImpl);
    if (!identity.handle) return { status: 'unverified', did, reason: 'no_handle' };
    if ((await resolveAtprotoHandle(identity.handle, fetchImpl)) !== did) {
      return { status: 'unverified', did, reason: 'handle_mismatch' };
    }
    if (!identity.pds || !(await hasAccountLinkRecord(identity.pds, did, account, fetchImpl))) {
      return { status: 'unverified', did, reason: 'no_link' };
    }
    return { status: 'verified', did, handle: identity.handle };
  } catch (error) {
    if (error instanceof UnavailableError || error instanceof TypeError) {
      return { status: 'unverified', did, reason: 'unavailable' };
    }
    throw error;
  }
}

/** Whether `did` is a syntactically valid DID of a method that AT Protocol supports. */
export function isAtprotoDid(did: string): boolean {
  return did.length <= 2048 && /^did:(plc|web):[A-Za-z0-9._:%-]*[A-Za-z0-9._-]$/.test(did);
}

/** Resolve `did` to its handle claim and PDS. */
export async function resolveAtprotoIdentity(
  did: string,
  fetchImpl: FetchLike = (input, init) => globalThis.fetch(input, init)
): Promise<AtprotoIdentity> {
  const document = await fetchJSON(didDocumentURL(did), fetchImpl);
  if (!isRecord(document) || document.id !== did)
    throw new UnavailableError('invalid DID document');
  const aliases = Array.isArray(document.alsoKnownAs) ? document.alsoKnownAs : [];
  const alias = aliases.find(
    (value): value is string => typeof value === 'string' && value.startsWith('at://')
  );
  const services = Array.isArray(document.service) ? document.service : [];
  const pdsService = services.find(
    (service) =>
      isRecord(service) &&
      (service.id === '#atproto_pds' || service.id === `${did}#atproto_pds`) &&
      service.type === 'AtprotoPersonalDataServer' &&
      typeof service.serviceEndpoint === 'string'
  ) as { serviceEndpoint: string } | undefined;
  return {
    did,
    handle: alias ? normalizeHandle(alias.slice('at://'.length)) : null,
    pds: pdsService && isHTTPSURL(pdsService.serviceEndpoint) ? pdsService.serviceEndpoint : null
  };
}

/**
 * Resolve `handle` to a DID through DNS (`_atproto` TXT record) and then
 * HTTPS (`/.well-known/atproto-did`). Returns `null` if neither method gives
 * a DID.
 */
export async function resolveAtprotoHandle(
  handle: string,
  fetchImpl: FetchLike = (input, init) => globalThis.fetch(input, init)
): Promise<string | null> {
  const normalized = normalizeHandle(handle);
  if (!isValidHandle(normalized)) return null;
  const dns = await resolveHandleDNS(normalized, fetchImpl).catch(() => null);
  if (dns) return dns;
  return resolveHandleHTTPS(normalized, fetchImpl).catch(() => null);
}

async function resolveHandleDNS(handle: string, fetchImpl: FetchLike): Promise<string | null> {
  const query = new URLSearchParams({ name: `_atproto.${handle}`, type: 'TXT' });
  const answer = await fetchJSON(`${DNS_OVER_HTTPS_URL}?${query}`, fetchImpl, {
    accept: 'application/dns-json'
  });
  if (!isRecord(answer) || !Array.isArray(answer.Answer)) return null;
  const dids = answer.Answer.flatMap((record) => {
    if (!isRecord(record) || record.type !== 16 || typeof record.data !== 'string') return [];
    const value = record.data.replace(/^"|"$/g, '').replaceAll('" "', '');
    return value.startsWith('did=') && isAtprotoDid(value.slice(4)) ? [value.slice(4)] : [];
  });
  // Several different DIDs make the handle ambiguous.
  return new Set(dids).size === 1 ? (dids[0] ?? null) : null;
}

async function resolveHandleHTTPS(handle: string, fetchImpl: FetchLike): Promise<string | null> {
  const response = await fetchWithTimeout(`https://${handle}/.well-known/atproto-did`, fetchImpl);
  if (!response.ok) return null;
  const did = (await response.text()).trim();
  return isAtprotoDid(did) ? did : null;
}

async function hasAccountLinkRecord(
  pds: string,
  did: string,
  account: ChattoAccountRef,
  fetchImpl: FetchLike
): Promise<boolean> {
  const query = new URLSearchParams({
    repo: did,
    collection: ATPROTO_ACCOUNT_LINK_COLLECTION,
    rkey: await accountLinkRecordKey(account)
  });
  const response = await fetchWithTimeout(
    `${pds.replace(/\/+$/, '')}/xrpc/com.atproto.repo.getRecord?${query}`,
    fetchImpl
  );
  if (response.status === 400 || response.status === 404) return false;
  if (!response.ok) throw new UnavailableError(`record lookup failed (${response.status})`);
  const body: unknown = await response.json();
  return (
    isRecord(body) &&
    isRecord(body.value) &&
    body.value.$type === ATPROTO_ACCOUNT_LINK_COLLECTION &&
    typeof body.uri === 'string' &&
    body.uri.startsWith(`at://${did}/`)
  );
}

function didDocumentURL(did: string): string {
  if (did.startsWith('did:plc:')) return `${PLC_DIRECTORY_URL}/${encodeURIComponent(did)}`;
  // did:web encodes a port as %3A. AT Protocol allows only host names.
  const host = decodeURIComponent(did.slice('did:web:'.length));
  if (host.includes(':') || host.includes('/')) throw new UnavailableError('unsupported did:web');
  return `https://${host}/.well-known/did.json`;
}

async function fetchJSON(
  url: string,
  fetchImpl: FetchLike,
  headers?: Record<string, string>
): Promise<unknown> {
  const response = await fetchWithTimeout(url, fetchImpl, headers);
  if (!response.ok) throw new UnavailableError(`request failed (${response.status})`);
  return response.json();
}

async function fetchWithTimeout(
  url: string,
  fetchImpl: FetchLike,
  headers?: Record<string, string>
): Promise<Response> {
  // Omit credentials and referrer: these requests go to third parties.
  return fetchImpl(url, {
    headers,
    credentials: 'omit',
    referrerPolicy: 'no-referrer',
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS)
  });
}

function canonicalOrigin(value: string): string {
  return new URL(value).origin;
}

function normalizeHandle(handle: string): string {
  return handle.trim().replace(/^@/, '').toLowerCase();
}

function isValidHandle(handle: string): boolean {
  return (
    handle.length <= 253 &&
    /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$/.test(handle)
  );
}

function isHTTPSURL(value: string): boolean {
  try {
    return new URL(value).protocol === 'https:';
  } catch {
    return false;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
