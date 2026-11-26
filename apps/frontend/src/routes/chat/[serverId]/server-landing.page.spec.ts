import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    serverId: 'origin' as string | null,
    lastRoomId: null as string | null
  }
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string> = {}) =>
    Object.entries(params).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (id: string) => (id === 'origin' ? '-' : 'book.example'),
  segmentToServerId: vi.fn(() => mocks.serverId)
}));

vi.mock('$lib/storage/lastRoom', () => ({
  getLastRoom: () => mocks.lastRoomId
}));

import { segmentToServerId } from '$lib/navigation';
import { load } from './+page';

async function routeLoad(path = 'https://chat.example.test/chat/-') {
  return load({
    params: { serverId: new URL(path).pathname.split('/').pop() },
    parent: async () => ({}),
    url: new URL(path)
  } as never);
}

describe('server landing load', () => {
  beforeEach(() => {
    mocks.serverId = 'origin';
    mocks.lastRoomId = null;
  });

  it.each([
    ['book.example', 'https://book.example'],
    ['bad:123', null]
  ])('shows the handoff state for an unsaved %s', async (segment, origin) => {
    mocks.serverId = null;
    await expect(routeLoad('https://app.example/chat/' + segment)).resolves.toEqual({ origin });
  });

  it('opens login when the origin shortcut has no registration', async () => {
    mocks.serverId = null;
    await expect(routeLoad()).rejects.toMatchObject({ status: 302, location: '/login' });
  });

  it.each([
    ['remote', 'book.example'],
    ['origin', '-']
  ])('opens the canonical segment for a saved %s', async (id, segment) => {
    mocks.serverId = id;
    await expect(routeLoad('https://app.example/chat/Book.Example')).rejects.toMatchObject({
      status: 302,
      location: '/chat/' + segment + '/overview'
    });
    expect(segmentToServerId).toHaveBeenLastCalledWith('book.example');
  });

  it('redirects to the remembered room before a page component mounts', async () => {
    mocks.lastRoomId = 'room-1';

    await expect(routeLoad()).rejects.toMatchObject({
      status: 302,
      location: '/chat/-/room-1'
    });
  });

  it('preserves query parameters while redirecting to the remembered room', async () => {
    mocks.lastRoomId = 'room-1';

    await expect(routeLoad('https://chat.example.test/chat/-?welcome=true')).rejects.toMatchObject({
      status: 302,
      location: '/chat/-/room-1?welcome=true'
    });
  });

  it('falls through to the server overview when no room is remembered', async () => {
    await expect(routeLoad()).rejects.toMatchObject({
      status: 302,
      location: '/chat/-/overview'
    });
  });

  it('preserves query parameters while redirecting to the server overview', async () => {
    await expect(routeLoad('https://chat.example.test/chat/-?welcome=true')).rejects.toMatchObject({
      status: 302,
      location: '/chat/-/overview?welcome=true'
    });
  });
});
