import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearPersistedAuthorization,
  loadPersistedAuthorization,
  savePersistedAuthorization
} from './persistedAuthorization';

const key = 'chatto:account-data:authorization';
const expected = {
  issuer: 'https://id.example',
  clientId: 'https://chat.example/oauth/client-metadata.json'
};
const authorization = {
  accessToken: 'access-token',
  expiresAt: 20_000,
  ...expected,
  accountId: 'account-123',
  providerLabel: 'Authling'
};

describe('persisted Authling account-data authorization', () => {
  const values = new Map<string, string>();

  beforeEach(() => {
    values.clear();
    vi.stubGlobal('localStorage', {
      getItem: (name: string) => values.get(name) ?? null,
      setItem: (name: string, value: string) => values.set(name, value),
      removeItem: (name: string) => values.delete(name)
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it('retains a valid short-lived grant in browser-local storage', () => {
    savePersistedAuthorization(authorization);

    expect(values.get(key)).toContain('access-token');
    expect(loadPersistedAuthorization(expected, 10_000)).toEqual(authorization);
  });

  it('removes an expired grant', () => {
    savePersistedAuthorization(authorization);

    expect(loadPersistedAuthorization(expected, 15_001)).toBeNull();
    expect(values.has(key)).toBe(false);
  });

  it('removes a grant belonging to another configured client', () => {
    savePersistedAuthorization(authorization);

    expect(
      loadPersistedAuthorization({ ...expected, clientId: 'https://other.example/client' }, 10_000)
    ).toBeNull();
    expect(values.has(key)).toBe(false);
  });

  it('can be explicitly cleared', () => {
    savePersistedAuthorization(authorization);
    clearPersistedAuthorization();

    expect(values.has(key)).toBe(false);
  });
});
