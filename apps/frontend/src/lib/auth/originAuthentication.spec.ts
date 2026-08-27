import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const {
  clearCachedUserMock,
  hasPendingReturnNavigationMock,
  invalidateAllMock,
  resumeReturnNavigationMock
} = vi.hoisted(() => ({
  clearCachedUserMock: vi.fn(),
  hasPendingReturnNavigationMock: vi.fn(),
  invalidateAllMock: vi.fn(),
  resumeReturnNavigationMock: vi.fn()
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
  clearCachedUser: clearCachedUserMock
}));

vi.mock('./returnNavigation', () => ({
  hasPendingReturnNavigation: hasPendingReturnNavigationMock,
  resumeReturnNavigation: resumeReturnNavigationMock
}));

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

  it('clears cached state before the cookie-backed route reload', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication()).resolves.toBe(false);

    expect(clearCachedUserMock).toHaveBeenCalledOnce();
    expect(invalidateAllMock).toHaveBeenCalledOnce();
    expect(clearCachedUserMock.mock.invocationCallOrder[0]).toBeLessThan(
      invalidateAllMock.mock.invocationCallOrder[0]
    );
    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });

  it('resumes a return path captured before route invalidation', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(true);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication()).resolves.toBe(true);

    expect(invalidateAllMock).toHaveBeenCalledOnce();
    expect(resumeReturnNavigationMock).toHaveBeenCalledOnce();
  });

  it('reports when authenticated route invalidation already navigated', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    invalidateAllMock.mockImplementation(async () => {
      window.location.pathname = '/chat';
    });
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication()).resolves.toBe(true);

    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });
});
