import { beforeEach, describe, expect, it, vi } from 'vitest';

const { csrfFetchMock, originServer } = vi.hoisted(() => ({
  csrfFetchMock: vi.fn(),
  originServer: {
    token: 'cht_AT_legacy',
    refreshToken: 'cht_RT_legacy'
  } as { token: string | null; refreshToken: string | null }
}));

vi.mock('./csrf', () => ({ csrfFetch: csrfFetchMock }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get originServer() {
      return originServer;
    }
  }
}));

import { revokeLegacyOriginBearerSession } from './originBearerMigration';

describe('revokeLegacyOriginBearerSession', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    originServer.token = 'cht_AT_legacy';
    originServer.refreshToken = 'cht_RT_legacy';
  });

  it('revokes the portable authority before local cookie adoption', async () => {
    csrfFetchMock.mockResolvedValue(new Response(null, { status: 200 }));

    await revokeLegacyOriginBearerSession();

    expect(csrfFetchMock).toHaveBeenCalledWith('/auth/browser/revoke-bearer-session', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Chatto-Authentication-Mode': 'cookie'
      },
      body: JSON.stringify({
        accessToken: 'cht_AT_legacy',
        refreshToken: 'cht_RT_legacy'
      })
    });
  });

  it('does nothing when the origin already uses only its cookie', async () => {
    originServer.token = null;
    originServer.refreshToken = null;

    await revokeLegacyOriginBearerSession();

    expect(csrfFetchMock).not.toHaveBeenCalled();
  });

  it('reports revocation failure so local authority is not abandoned', async () => {
    csrfFetchMock.mockResolvedValue(new Response(null, { status: 503 }));

    await expect(revokeLegacyOriginBearerSession()).rejects.toThrow(
      'Origin bearer-session revocation failed (503)'
    );
  });
});
