import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { CurrentUser } from '$lib/auth/loadAuth';

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  hasPendingReturnNavigation: vi.fn(),
  resolveLastPosition: vi.fn()
}));

vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '')
}));
vi.mock('$lib/auth/returnNavigation', () => ({
  hasPendingReturnNavigation: mocks.hasPendingReturnNavigation
}));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    originServer: { id: 'origin' },
    servers: [{ id: 'origin' }],
    tryGetStore: () => ({ permissions: { loaded: false } })
  }
}));
vi.mock('$lib/storage/lastRoom', () => ({ resolveLastPosition: mocks.resolveLastPosition }));

import ChatLanding from './+page.svelte';

const user: CurrentUser = {
  id: 'user-1',
  login: 'user',
  displayName: 'User',
  avatarUrl: null,
  customStatus: null,
  presenceStatus: PresenceStatus.ONLINE,
  hasVerifiedEmail: true,
  hasPassword: true,
  viewerCanDeleteAccount: true,
  lastLoginChange: null,
  settings: null
};

describe('chat landing page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.hasPendingReturnNavigation.mockReturnValue(false);
    mocks.resolveLastPosition.mockReturnValue(null);
  });

  it('navigates before the first viewer projection loads permissions', async () => {
    render(ChatLanding, {
      props: {
        data: { user, serverInfo: null, serverInfoLoaded: true, welcome: false }
      }
    });

    await vi.waitFor(() =>
      expect(mocks.goto).toHaveBeenCalledWith('/chat/origin', {
        replaceState: true,
        state: {}
      })
    );
  });
});
