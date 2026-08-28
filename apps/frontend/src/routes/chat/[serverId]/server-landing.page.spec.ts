import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    serverId: 'origin' as string | null,
    lastRoomId: null as string | null
  }
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    Object.entries(params).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));

vi.mock('$lib/navigation', () => ({
  segmentToServerId: () => mocks.serverId
}));

vi.mock('$lib/storage/lastRoom', () => ({
  getLastRoom: () => mocks.lastRoomId
}));

import { load } from './+page';

async function routeLoad(path = 'https://chat.example.test/chat/-') {
  return load({ params: { serverId: '-' }, url: new URL(path) } as never);
}

describe('server landing load', () => {
  beforeEach(() => {
    mocks.serverId = 'origin';
    mocks.lastRoomId = null;
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
