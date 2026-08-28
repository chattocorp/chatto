import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  isReturnNavigationInProgress: vi.fn(),
  takeReturnNavigationTarget: vi.fn()
}));

vi.mock('$lib/auth/returnNavigation', () => mocks);

import { load } from './+page';

async function expectRedirect(
  url: string,
  user: { id: string } | null,
  location: string
): Promise<void> {
  await expect(
    load({
      parent: async () => ({ user }),
      url: new URL(url)
    } as never)
  ).rejects.toMatchObject({ status: 302, location });
}

describe('chat landing load', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.isReturnNavigationInProgress.mockReturnValue(false);
    mocks.takeReturnNavigationTarget.mockReturnValue(null);
  });

  it('gives a queued return path precedence over the default origin route', async () => {
    mocks.takeReturnNavigationTarget.mockReturnValue('/chat/-/settings?tab=profile');

    await expectRedirect(
      'https://chat.example.test/chat',
      { id: 'user-1' },
      '/chat/-/settings?tab=profile'
    );
  });

  it('does not compete with a return navigation already in progress', async () => {
    mocks.isReturnNavigationInProgress.mockReturnValue(true);

    await expect(
      load({
        parent: async () => ({ user: { id: 'user-1' } }),
        url: new URL('https://chat.example.test/chat')
      } as never)
    ).resolves.toEqual({});
    expect(mocks.takeReturnNavigationTarget).not.toHaveBeenCalled();
  });

  it('redirects authenticated users to the origin segment without waiting for a viewer projection', async () => {
    await expectRedirect('https://chat.example.test/chat', { id: 'user-1' }, '/chat/-');
  });

  it('preserves the welcome signal for the origin route', async () => {
    await expectRedirect(
      'https://chat.example.test/chat?welcome=true',
      { id: 'user-1' },
      '/chat/-?welcome=true'
    );
  });
});
