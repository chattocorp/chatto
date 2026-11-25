import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const {
  resetAccountMock,
  hasPendingReturnNavigationMock,
  invalidateMock,
  resumeReturnNavigationMock
} = vi.hoisted(() => ({
  resetAccountMock: vi.fn(),
  hasPendingReturnNavigationMock: vi.fn(),
  invalidateMock: vi.fn(),
  resumeReturnNavigationMock: vi.fn()
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    originServer: { id: 'origin' },
    getStore: () => ({ currentUser: { reset: resetAccountMock } })
  }
}));

vi.mock('$app/navigation', () => ({
  invalidate: invalidateMock
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
    invalidateMock.mockResolvedValue(undefined);
    resumeReturnNavigationMock.mockResolvedValue(true);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('invalidates old account requests before the cookie-backed route reload', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication()).resolves.toBe(false);

    expect(resetAccountMock).toHaveBeenCalledOnce();
    expect(invalidateMock).toHaveBeenCalledWith('chatto:origin-session');
    expect(resetAccountMock.mock.invocationCallOrder[0]).toBeLessThan(
      invalidateMock.mock.invocationCallOrder[0]
    );
    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });

  it('resumes a return path captured before route invalidation', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(true);
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication()).resolves.toBe(true);

    expect(invalidateMock).toHaveBeenCalledWith('chatto:origin-session');
    expect(resumeReturnNavigationMock).toHaveBeenCalledOnce();
  });

  it('reports when authenticated route invalidation already navigated', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(false);
    invalidateMock.mockImplementation(async () => {
      window.location.pathname = '/chat';
    });
    const { completeOriginAuthentication } = await loadModule();

    await expect(completeOriginAuthentication()).resolves.toBe(true);

    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });

  it('leaves copied return navigation to the opening tab during provider completion', async () => {
    hasPendingReturnNavigationMock.mockReturnValue(true);
    const { completeOriginAuthentication } = await loadModule();
    await expect(completeOriginAuthentication({ resumeReturnNavigation: false })).resolves.toBe(
      false
    );
    expect(resetAccountMock).toHaveBeenCalledOnce();
    expect(invalidateMock).toHaveBeenCalledWith('chatto:origin-session');
    expect(resumeReturnNavigationMock).not.toHaveBeenCalled();
  });
});
