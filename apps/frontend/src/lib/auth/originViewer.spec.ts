import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';

const mocks = vi.hoisted(() => ({ viewer: vi.fn(), revoke: vi.fn(), migrate: vi.fn() }));
vi.mock('$lib/api-client/viewer', () => ({ getCurrentUserViaConnect: mocks.viewer }));
vi.mock('./originBearerMigration', () => ({ revokeLegacyOriginBearerSession: mocks.revoke }));
vi.mock('./legacyCookieMigration', () => ({ migrateLegacyOriginCookieSession: mocks.migrate }));

import { getOriginViewer } from './originViewer';

const config = { baseUrl: '/api/connect', bearerToken: 'legacy-token' };
const user = { id: 'U1', login: 'alice' };
const rejected = new ConnectError('authentication required', Code.Unauthenticated);

describe('origin viewer transport', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    mocks.revoke.mockResolvedValue(undefined);
    mocks.migrate.mockResolvedValue(false);
  });

  it('uses only the cookie and revokes legacy bearer authority before returning', async () => {
    mocks.viewer.mockResolvedValue(user);
    expect(await getOriginViewer(config)).toBe(user);
    expect(mocks.viewer).toHaveBeenCalledWith({ baseUrl: '/api/connect', bearerToken: null });
    expect(mocks.revoke).toHaveBeenCalledOnce();
  });

  it('retries transient failures once and then reports the failure to the account owner', async () => {
    mocks.viewer.mockRejectedValue(new Error('offline'));
    await expect(getOriginViewer(config)).rejects.toThrow('offline');
    expect(mocks.viewer).toHaveBeenCalledTimes(2);
    expect(mocks.migrate).not.toHaveBeenCalled();
  });

  it.each([false, true])(
    'migrates a legacy cookie after transient error: %s',
    async (transient) => {
      if (transient) mocks.viewer.mockRejectedValueOnce(new Error('offline'));
      mocks.viewer.mockRejectedValueOnce(rejected).mockResolvedValueOnce(user);
      mocks.migrate.mockResolvedValueOnce(true);
      expect(await getOriginViewer(config)).toBe(user);
      expect(mocks.migrate).toHaveBeenCalledOnce();
      expect(mocks.viewer).toHaveBeenCalledTimes(transient ? 3 : 2);
    }
  );

  it('reports rejection only after migration and legacy revocation complete', async () => {
    mocks.viewer.mockRejectedValue(rejected);
    await expect(getOriginViewer(config)).rejects.toBe(rejected);
    expect(mocks.migrate).toHaveBeenCalledOnce();
    expect(mocks.revoke).toHaveBeenCalledOnce();
  });

  it('treats migration failure as transient and preserves legacy authority', async () => {
    mocks.viewer.mockRejectedValue(rejected);
    mocks.migrate.mockRejectedValue(new Error('migration unavailable'));
    await expect(getOriginViewer(config)).rejects.toThrow('migration unavailable');
    expect(mocks.migrate).toHaveBeenCalledTimes(2);
    expect(mocks.revoke).not.toHaveBeenCalled();
  });

  it.each([true, false])(
    'does not publish an account if revocation fails, cookie valid: %s',
    async (valid) => {
      if (valid) mocks.viewer.mockResolvedValue(user);
      else mocks.viewer.mockRejectedValue(rejected);
      mocks.revoke.mockRejectedValue(new Error('revocation unavailable'));
      await expect(getOriginViewer(config)).rejects.toThrow('revocation unavailable');
    }
  );

  it('does not treat an unavailable response containing auth wording as rejection', async () => {
    const unavailable = new ConnectError(
      'authentication required: storage unavailable',
      Code.Unavailable
    );
    mocks.viewer.mockRejectedValueOnce(unavailable).mockResolvedValueOnce(user);
    expect(await getOriginViewer(config)).toBe(user);
    expect(mocks.migrate).not.toHaveBeenCalled();
  });
});
