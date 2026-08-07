import { beforeEach, describe, expect, it, vi } from 'vitest';

const {
  authenticateOriginMock,
  clearCachedUserMock,
  hasPendingReturnNavigationMock,
  invalidateAllMock,
  resumeReturnNavigationMock
} = vi.hoisted(() => ({
  authenticateOriginMock: vi.fn(),
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
    authenticateOrigin: authenticateOriginMock
  }
}));

vi.mock('./loadAuth', () => ({
  clearCachedUser: clearCachedUserMock
}));

vi.mock('./returnNavigation', () => ({
  hasPendingReturnNavigation: hasPendingReturnNavigationMock,
  resumeReturnNavigation: resumeReturnNavigationMock
}));

const user = {
  id: 'user-1',
  login: 'alice'
};

async function loadModule() {
  vi.resetModules();
  return import('./originAuthentication');
}

describe('completeOriginAuthentication', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    invalidateAllMock.mockResolvedValue(undefined);
    resumeReturnNavigationMock.mockResolvedValue(true);
  });

  it('installs only origin authentication and refreshes route data', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication('origin-token', user)).resolves.toBe(false);

    expect(authenticateOriginMock).toHaveBeenCalledWith('origin-token', user);
    expect(clearCachedUserMock).toHaveBeenCalledOnce();
    expect(invalidateAllMock).toHaveBeenCalledOnce();
    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });

  it('resumes a return path captured before route invalidation', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(true);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication('origin-token', user)).resolves.toBe(true);

    expect(invalidateAllMock).toHaveBeenCalledOnce();
    expect(resumeReturnNavigationMock).toHaveBeenCalledOnce();
  });
});
