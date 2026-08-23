import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const {
  clearCachedUserMock,
  hasPendingReturnNavigationMock,
  invalidateAllMock,
  resumeReturnNavigationMock,
  stagePendingOriginAuthenticationMock
} = vi.hoisted(() => ({
  clearCachedUserMock: vi.fn(),
  hasPendingReturnNavigationMock: vi.fn(),
  invalidateAllMock: vi.fn(),
  resumeReturnNavigationMock: vi.fn(),
  stagePendingOriginAuthenticationMock: vi.fn()
}));

vi.mock('$app/navigation', () => ({
  invalidateAll: invalidateAllMock
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    originServer: { id: 'origin' }
  }
}));

vi.mock('./loadAuth', () => ({
  clearCachedUser: clearCachedUserMock,
  stagePendingOriginAuthentication: stagePendingOriginAuthenticationMock
}));

vi.mock('./returnNavigation', () => ({
  hasPendingReturnNavigation: hasPendingReturnNavigationMock,
  resumeReturnNavigation: resumeReturnNavigationMock
}));

const credentials = {
  token: 'origin-token',
  refreshToken: 'origin-refresh-token',
  expiresIn: 900,
  refreshTokenExpiresIn: 7776000,
  oauthClientId: null
};

async function loadModule() {
  vi.resetModules();
  return import('./originAuthentication');
}

describe('completeOriginAuthentication', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('window', {
      location: { pathname: '/login', search: '', hash: '' }
    });
    invalidateAllMock.mockResolvedValue(undefined);
    resumeReturnNavigationMock.mockResolvedValue(true);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('stages the direct bearer without installing it before route invalidation', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication(credentials)).resolves.toBe(false);

    expect(clearCachedUserMock).toHaveBeenCalledOnce();
    expect(stagePendingOriginAuthenticationMock).toHaveBeenCalledWith(credentials);
    expect(invalidateAllMock).toHaveBeenCalledOnce();
    expect(clearCachedUserMock.mock.invocationCallOrder[0]).toBeLessThan(
      stagePendingOriginAuthenticationMock.mock.invocationCallOrder[0]
    );
    expect(stagePendingOriginAuthenticationMock.mock.invocationCallOrder[0]).toBeLessThan(
      invalidateAllMock.mock.invocationCallOrder[0]
    );
    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });

  it('can complete a cookie-only response without a bearer fallback', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication(null)).resolves.toBe(false);

    expect(stagePendingOriginAuthenticationMock).toHaveBeenCalledWith(null);
    expect(invalidateAllMock).toHaveBeenCalledOnce();
  });

  it('resumes a return path captured before route invalidation', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(true);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication(credentials)).resolves.toBe(true);

    expect(invalidateAllMock).toHaveBeenCalledOnce();
    expect(resumeReturnNavigationMock).toHaveBeenCalledOnce();
  });

  it('reports when authenticated route invalidation already navigated', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    invalidateAllMock.mockImplementation(async () => {
      window.location.pathname = '/chat';
    });
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication(credentials)).resolves.toBe(true);

    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });
});
