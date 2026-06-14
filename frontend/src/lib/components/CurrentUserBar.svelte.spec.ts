import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { PresenceStatus } from '$lib/gql/graphql';
import CurrentUserBarTestHarness from './CurrentUserBarTestHarness.svelte';

const currentUserState = vi.hoisted(() => ({
  user: null as {
    id: string;
    login: string;
    displayName: string;
    avatarUrl: string | null;
    presenceStatus: PresenceStatus;
    hasVerifiedEmail: boolean;
    settings: null;
  } | null
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: () => true,
    tryGetStore: () => ({
      currentUser: currentUserState
    })
  }
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback
}));

describe('CurrentUserBar', () => {
  beforeEach(() => {
    currentUserState.user = {
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      presenceStatus: PresenceStatus.Offline,
      hasVerifiedEmail: true,
      settings: null
    };
  });

  it('uses the seeded presence cache instead of the first-login offline fallback', () => {
    const { container } = render(CurrentUserBarTestHarness);

    expect(q(container, '[aria-label="Online"]')).toBeTruthy();
    expect(q(container, '[aria-label="Offline"]')).toBeFalsy();
    expect(container.textContent).toContain('Alice');
    expect(container.textContent).toContain('@alice');
  });
});
