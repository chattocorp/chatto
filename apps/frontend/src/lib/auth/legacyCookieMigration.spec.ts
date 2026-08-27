import { afterEach, describe, expect, it, vi } from 'vitest';
import { migrateLegacyOriginCookieSession } from './legacyCookieMigration';

describe('migrateLegacyOriginCookieSession', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('requests the one-time migration with the browser authentication proof', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(migrateLegacyOriginCookieSession()).resolves.toBe(true);
    expect(fetchMock).toHaveBeenCalledWith('/auth/browser/session/migrate', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Chatto-Authentication-Mode': 'cookie'
      },
      body: '{}'
    });
  });

  it('reports that no migratable cookie remains', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 401 }))
    );

    await expect(migrateLegacyOriginCookieSession()).resolves.toBe(false);
  });

  it('keeps temporary failures distinct from missing authentication', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 503 }))
    );

    await expect(migrateLegacyOriginCookieSession()).rejects.toThrow(
      'Legacy browser-session migration failed (503)'
    );
  });
});
