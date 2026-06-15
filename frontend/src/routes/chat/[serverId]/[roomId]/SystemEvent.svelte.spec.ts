import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import SystemEvent from './SystemEvent.svelte';

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_userId: string, fallback: unknown) => fallback
  })
}));

function systemEvent(
  typename: 'CallStartedEvent' | 'CallEndedEvent',
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
      roomId: 'room-1',
      callId: 'call-1'
    }
  } as unknown as RoomEventViewFragment;
}

describe('SystemEvent', () => {
  it('renders call start copy with the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent('CallStartedEvent', 'Alice') }
    });

    expect(container.textContent).toContain('Alice started a call');
  });

  it('renders call end copy without the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent('CallEndedEvent', 'Alice') }
    });

    expect(container.textContent).toContain('The active call has ended');
    expect(container.textContent).not.toContain('Alice');
  });
});
