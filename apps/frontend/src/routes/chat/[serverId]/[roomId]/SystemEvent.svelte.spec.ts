import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import SystemEvent from './SystemEvent.svelte';

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_userId: string, fallback: unknown) => fallback
  })
}));

function systemEvent(
  typename: 'UserJoinedRoomEvent' | 'UserLeftRoomEvent',
  actorName = 'Alice'
): RoomEventViewFragment {
  return {
    id: `evt-${typename}`,
    createdAt: '2026-06-15T12:00:00Z',
    actorId: 'user-1',
    actor: {
      id: 'user-1',
      login: 'alice',
      displayName: actorName,
      avatarUrl: null,
      presenceStatus: null
    },
    event: {
      __typename: typename,
      roomId: 'room-1'
    }
  } as unknown as RoomEventViewFragment;
}

describe('SystemEvent', () => {
  it('renders member join copy with the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent('UserJoinedRoomEvent', 'Alice') }
    });

    expect(container.textContent).toContain('Alice joined the room');
  });

  it('renders member leave copy with the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent('UserLeftRoomEvent', 'Alice') }
    });

    expect(container.textContent).toContain('Alice left the room');
  });
});
