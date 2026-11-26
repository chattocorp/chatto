import { beforeEach, describe, expect, it } from 'vitest';
import { serverStorageKey } from '../storage/serverStorage.js';
import { SessionPersistence, restorePersistedServerState } from './sessionPersistence.js';
import { emptyServerSession, type RegisteredServer } from './sessions.js';

const STORAGE_KEY = 'chatto:instances';

function authenticationStorageKey(id: string) {
  return serverStorageKey(id, 'authentication');
}

function makeServer(fields: Partial<RegisteredServer>): RegisteredServer {
  return {
    ...emptyServerSession(),
    id: 'remote',
    name: 'Remote',
    url: 'https://remote.example',
    addedAt: 1,
    iconUrl: null,
    ...fields
  };
}

beforeEach(() => localStorage.clear());

describe('localStorage persistence', () => {
  it('loads instances from localStorage on construction', () => {
    const server = makeServer({ id: 'persisted', name: 'Persisted' });
    localStorage.setItem(STORAGE_KEY, JSON.stringify([server]));

    const restored = restorePersistedServerState();

    expect(restored.registrations).toEqual([
      expect.objectContaining({ id: 'persisted', name: 'Persisted' })
    ]);
    expect(restored.sessions).toEqual([
      ['persisted', expect.objectContaining({ token: server.token, userId: server.userId })]
    ]);
  });

  it('restores per-server authentication instead of stale combined credentials', () => {
    const server = makeServer({
      id: 'persisted',
      token: 'stale-access',
      refreshToken: 'stale-refresh',
      refreshRequestId: 'stale-request-id'
    });
    localStorage.setItem(STORAGE_KEY, JSON.stringify([server]));
    localStorage.setItem(
      authenticationStorageKey('persisted'),
      JSON.stringify({
        version: 1,
        token: 'rotated-access',
        refreshToken: 'rotated-refresh',
        accessTokenExpiresAt: 20_000,
        refreshTokenExpiresAt: 30_000,
        oauthClientId: null,
        refreshRequestId: 'rotated-request-id',
        reauthRequiredAt: null
      })
    );

    const restored = restorePersistedServerState();

    expect(restored.sessions[0][1]).toMatchObject({
      token: 'rotated-access',
      refreshToken: 'rotated-refresh',
      refreshRequestId: 'rotated-request-id'
    });
  });

  it('clears retired account-data browser storage', () => {
    localStorage.setItem('chatto:account-data:authorization', 'grant');
    localStorage.setItem('chatto:account-data:device-id', 'device');
    localStorage.setItem('chatto:account-data:tinybase', 'cache');

    restorePersistedServerState();

    expect(localStorage.getItem('chatto:account-data:authorization')).toBeNull();
    expect(localStorage.getItem('chatto:account-data:device-id')).toBeNull();
    expect(localStorage.getItem('chatto:account-data:tinybase')).toBeNull();
  });

  it('rewrites retired sync provenance as device-local storage', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify([{ ...makeServer({ id: 'migrated' }), source: 'synced' }])
    );

    restorePersistedServerState();

    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]')[0]).not.toHaveProperty('source');
  });

  it('handles corrupted localStorage gracefully', () => {
    localStorage.setItem(STORAGE_KEY, 'not valid json!!!');

    expect(restorePersistedServerState()).toEqual({ registrations: [], sessions: [] });
  });

  it('handles non-array localStorage gracefully', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ not: 'an array' }));

    expect(restorePersistedServerState()).toEqual({ registrations: [], sessions: [] });
  });

  it.each([[null], [1], [{ id: 'partial' }]])(
    'handles malformed entries in the persisted array: %j',
    (value) => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(value));

      expect(restorePersistedServerState()).toEqual({ registrations: [], sessions: [] });
    }
  );
});

describe('stored bearer credentials', () => {
  const owner = {
    ...emptyServerSession(),
    token: 'access',
    refreshToken: 'refresh',
    bearerSessionId: 'first'
  };

  it('distinguishes absent shared storage from a client with no shared credentials', () => {
    expect(
      new SessionPersistence({ deviceStorage: false }).bearerSessionChange('remote', owner)
    ).toEqual({ action: 'keep' });
    expect(
      new SessionPersistence({ deviceStorage: true }).bearerSessionChange('remote', owner)
    ).toEqual({ action: 'discard' });
  });

  it('migrates a browser catalogue but leaves a host-owned legacy catalogue read-only', () => {
    const server = makeServer(owner);
    localStorage.setItem(STORAGE_KEY, JSON.stringify([server]));
    const browser = new SessionPersistence({ deviceStorage: true });
    expect(browser.restore().sessions[0][1].token).toBe(owner.token);
    expect(browser.readAuthentication(server.id)?.token).toBe(owner.token);

    const host = new SessionPersistence({
      deviceStorage: false,
      persistence: {
        shared: false,
        readServers: () => [server],
        readAuthentication: () => undefined,
        writeServers: () => {
          throw new Error('Unexpected catalogue write');
        },
        writeAuthentication: () => {
          throw new Error('Unexpected credential write');
        }
      }
    });
    expect(host.restore().sessions[0][1].token).toBe(owner.token);
  });

  it('clears corrupt credentials even when the catalogue contains a usable token', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify([makeServer(owner)]));
    localStorage.setItem(authenticationStorageKey('remote'), '{');
    const storage = new SessionPersistence({ deviceStorage: true });
    expect(storage.restore().sessions[0][1].token).toBeNull();
    expect(storage.bearerSessionChange('remote', owner)).toEqual({ action: 'discard' });
  });

  it('allows a same-sign-in rotation and rejects a stale patch after a new sign-in', () => {
    const storage = new SessionPersistence({ deviceStorage: true });
    const rotated = { ...owner, token: 'rotated-access', refreshToken: 'rotated-refresh' };
    storage.writeSession('remote', rotated);
    expect(storage.viewerSessionChanged('remote', owner)).toBe(false);
    expect(storage.bearerSessionChange('remote', owner)).toMatchObject({
      action: 'adopt',
      credentials: { token: rotated.token }
    });
    storage.writeSession('remote', { ...rotated, bearerSessionId: 'second' });
    expect(storage.viewerSessionChanged('remote', owner)).toBe(true);
    expect(storage.bearerSessionChange('remote', owner)).toEqual({ action: 'discard' });
    expect(storage.patchAuthentication('remote', owner, { reauthRequiredAt: 12 })).toBe(false);
    expect(storage.readAuthentication('remote')?.reauthRequiredAt).toBeNull();
  });
});
