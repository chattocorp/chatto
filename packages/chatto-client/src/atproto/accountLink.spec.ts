import { describe, expect, it, vi } from 'vitest';
import {
  ATPROTO_ACCOUNT_LINK_COLLECTION,
  accountLinkRecordKey,
  createAtprotoLinkVerifier,
  deleteAccountLinkRecord,
  isAtprotoDid,
  resolveAtprotoHandle,
  writeAccountLinkRecord,
  type FetchLike
} from './accountLink.js';

const DID = 'did:plc:z72i7hdynmk6r22z27h6tvur';
const PDS = 'https://pds.example';
const account = { serverOrigin: 'https://chat.example/', userId: 'U1' };

type Network = {
  didDocument?: unknown;
  dnsTxt?: string[];
  wellKnownDid?: string;
  record?: { status: number; body?: unknown };
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' }
  });
}

function fakeNetwork(network: Network) {
  const requests: string[] = [];
  const fetchImpl = vi.fn<FetchLike>(async (input, init) => {
    requests.push(input);
    expect(init?.credentials).toBe('omit');
    const url = new URL(input);
    if (url.origin === 'https://plc.directory') {
      return network.didDocument ? json(network.didDocument) : json({}, 404);
    }
    if (url.origin === 'https://cloudflare-dns.com') {
      return json({
        Status: 0,
        Answer: (network.dnsTxt ?? []).map((data) => ({ type: 16, data: `"${data}"` }))
      });
    }
    if (url.pathname === '/.well-known/atproto-did') {
      return network.wellKnownDid
        ? new Response(network.wellKnownDid)
        : new Response('', { status: 404 });
    }
    if (url.origin === PDS && url.pathname === '/xrpc/com.atproto.repo.getRecord') {
      const record = network.record ?? { status: 400, body: { error: 'RecordNotFound' } };
      return json(record.body ?? {}, record.status);
    }
    throw new TypeError(`unexpected request ${input}`);
  });
  return { fetchImpl, requests };
}

function didDocument(handle = 'alice.example') {
  return {
    id: DID,
    alsoKnownAs: [`at://${handle}`],
    service: [{ id: '#atproto_pds', type: 'AtprotoPersonalDataServer', serviceEndpoint: PDS }]
  };
}

async function linkRecord() {
  const rkey = await accountLinkRecordKey(account);
  return {
    status: 200,
    body: {
      uri: `at://${DID}/${ATPROTO_ACCOUNT_LINK_COLLECTION}/${rkey}`,
      value: { $type: ATPROTO_ACCOUNT_LINK_COLLECTION, createdAt: '2026-11-11T00:00:00Z' }
    }
  };
}

describe('accountLinkRecordKey', () => {
  it('hashes the canonical origin and user ID', async () => {
    const key = await accountLinkRecordKey(account);
    expect(key).toMatch(/^[0-9a-f]{64}$/);
    await expect(
      accountLinkRecordKey({ serverOrigin: 'https://CHAT.example', userId: 'U1' })
    ).resolves.toBe(key);
    await expect(
      accountLinkRecordKey({ serverOrigin: 'https://other.example', userId: 'U1' })
    ).resolves.not.toBe(key);
    await expect(
      accountLinkRecordKey({ serverOrigin: 'https://chat.example', userId: 'U2' })
    ).resolves.not.toBe(key);
  });
});

describe('isAtprotoDid', () => {
  it.each([
    [DID, true],
    ['did:web:example.com', true],
    ['did:key:z6Mk', false],
    ['alice.bsky.social', false],
    ['did:plc:abc:', false]
  ])('%s → %s', (did, valid) => {
    expect(isAtprotoDid(did)).toBe(valid);
  });
});

describe('createAtprotoLinkVerifier', () => {
  it('verifies a DID with a matching handle and link record', async () => {
    const { fetchImpl } = fakeNetwork({
      didDocument: didDocument(),
      dnsTxt: [`did=${DID}`],
      record: await linkRecord()
    });
    const verifier = createAtprotoLinkVerifier(fetchImpl);
    await expect(verifier.verify(DID, account)).resolves.toEqual({
      status: 'verified',
      did: DID,
      handle: 'alice.example'
    });
  });

  it('falls back to HTTPS handle resolution', async () => {
    const { fetchImpl } = fakeNetwork({
      didDocument: didDocument(),
      wellKnownDid: DID,
      record: await linkRecord()
    });
    await expect(createAtprotoLinkVerifier(fetchImpl).verify(DID, account)).resolves.toMatchObject({
      status: 'verified'
    });
  });

  it('rejects a link record for another account', async () => {
    const { fetchImpl } = fakeNetwork({ didDocument: didDocument(), dnsTxt: [`did=${DID}`] });
    await expect(createAtprotoLinkVerifier(fetchImpl).verify(DID, account)).resolves.toEqual({
      status: 'unverified',
      did: DID,
      reason: 'no_link'
    });
  });

  it('rejects a handle that does not point back to the DID', async () => {
    const { fetchImpl, requests } = fakeNetwork({
      didDocument: didDocument('famous.example'),
      dnsTxt: ['did=did:plc:someoneelse'],
      record: await linkRecord()
    });
    await expect(createAtprotoLinkVerifier(fetchImpl).verify(DID, account)).resolves.toMatchObject({
      reason: 'handle_mismatch'
    });
    expect(requests.some((url) => url.startsWith(PDS))).toBe(false);
  });

  it('rejects ambiguous DNS records', async () => {
    const { fetchImpl } = fakeNetwork({ dnsTxt: [`did=${DID}`, 'did=did:plc:other'] });
    await expect(resolveAtprotoHandle('alice.example', fetchImpl)).resolves.toBeNull();
  });

  it('rejects invalid DIDs without network requests', async () => {
    const { fetchImpl } = fakeNetwork({});
    await expect(
      createAtprotoLinkVerifier(fetchImpl).verify('did:key:abc', account)
    ).resolves.toMatchObject({
      reason: 'invalid_did'
    });
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('caches results but retries unavailable lookups', async () => {
    const network: Network = {};
    const { fetchImpl } = fakeNetwork(network);
    const verifier = createAtprotoLinkVerifier(fetchImpl);
    await expect(verifier.verify(DID, account)).resolves.toMatchObject({ reason: 'unavailable' });

    network.didDocument = didDocument();
    network.dnsTxt = [`did=${DID}`];
    network.record = await linkRecord();
    await expect(verifier.verify(DID, account)).resolves.toMatchObject({ status: 'verified' });
    const calls = fetchImpl.mock.calls.length;
    await verifier.verify(DID, account);
    expect(fetchImpl.mock.calls.length).toBe(calls);

    verifier.forget(DID, account);
    await verifier.verify(DID, account);
    expect(fetchImpl.mock.calls.length).toBeGreaterThan(calls);
  });
});

describe('account link record writes', () => {
  it('puts and deletes the record in the session repository', async () => {
    const fetchHandler = vi.fn(async () => json({}));
    const session = { did: DID, fetchHandler };
    await writeAccountLinkRecord(session, account);
    await deleteAccountLinkRecord(session, account);
    const rkey = await accountLinkRecordKey(account);

    const [putPath, putInit] = fetchHandler.mock.calls[0] as unknown as [string, RequestInit];
    expect(putPath).toBe('/xrpc/com.atproto.repo.putRecord');
    expect(JSON.parse(putInit.body as string)).toMatchObject({
      repo: DID,
      collection: ATPROTO_ACCOUNT_LINK_COLLECTION,
      rkey,
      record: { $type: ATPROTO_ACCOUNT_LINK_COLLECTION }
    });
    const [deletePath, deleteInit] = fetchHandler.mock.calls[1] as unknown as [string, RequestInit];
    expect(deletePath).toBe('/xrpc/com.atproto.repo.deleteRecord');
    expect(JSON.parse(deleteInit.body as string)).toEqual({
      repo: DID,
      collection: ATPROTO_ACCOUNT_LINK_COLLECTION,
      rkey
    });
  });

  it('reports a failed write', async () => {
    const session = { did: DID, fetchHandler: async () => json({ error: 'Forbidden' }, 403) };
    await expect(writeAccountLinkRecord(session, account)).rejects.toThrow('HTTP 403');
  });
});
