import { describe, it, expect, vi, afterEach } from 'vitest';
import { flushSync } from 'svelte';
import { makeSubject, type Source, type Subject } from 'wonka';
import type { Client } from '@urql/svelte';
import { ServerStateStore } from './store.svelte';
import { eventBusManager } from './eventBus.svelte';
import type { GraphQLClient } from './graphqlClient.svelte';
import type { RegisteredServer } from './registry.svelte';

class FakeGqlClient {
  reconnectCount = $state(0);
  client: Client;
  subject: Subject<{ data?: unknown; error?: unknown }>;
  query = vi.fn();
  results: unknown[];

  constructor(results: unknown[]) {
    this.results = results;
    this.subject = makeSubject<{ data?: unknown; error?: unknown }>();
    this.query.mockImplementation(() => {
      const data = this.results.shift() ?? null;
      return {
        toPromise: vi.fn().mockResolvedValue({ data, error: null })
      };
    });
    this.client = {
      query: this.query,
      mutation: vi.fn(),
      subscription: vi.fn(() => this.subject.source as unknown as Source<unknown>)
    } as unknown as Client;
  }

}

const registered: RegisteredServer = {
  id: 'store-event-test',
  url: 'https://store-event.test',
  name: 'Store Event Test',
  iconUrl: null,
  token: 'remote-token',
  userId: 'U1',
  userLogin: 'alice',
  userDisplayName: 'Alice',
  userAvatarUrl: null,
  addedAt: 1
};

afterEach(() => {
  eventBusManager.stopBus(registered.id);
});

describe('ServerStateStore live server updates', () => {
  it('refreshes public profile and authenticated settings on ServerUpdatedEvent', async () => {
    const fake = new FakeGqlClient([
      {
        server: {
          pushNotificationsEnabled: false,
          vapidPublicKey: null,
          livekitUrl: null,
          videoProcessingEnabled: false,
          maxUploadSize: 25,
          maxVideoUploadSize: 25,
          messageEditWindowSeconds: 3600,
          profile: {
            motd: null
          }
        }
      },
      { server: { rooms: [] } },
      { server: { rooms: [] } },
      { server: { rooms: [], roomGroups: [] } },
      {
        server: {
          directRegistrationEnabled: false,
          profile: {
            name: 'Fresh Name',
            welcomeMessage: 'Fresh welcome',
            description: 'Fresh description',
            logoUrl: 'https://cdn/icon.webp',
            bannerUrl: 'https://cdn/banner.webp'
          }
        }
      },
      {
        server: {
          pushNotificationsEnabled: true,
          vapidPublicKey: 'vapid',
          livekitUrl: 'wss://livekit',
          videoProcessingEnabled: true,
          maxUploadSize: 100,
          maxVideoUploadSize: 200,
          messageEditWindowSeconds: 120,
          profile: {
            motd: 'Fresh MOTD'
          }
        }
      }
    ]);
    const store = new ServerStateStore(registered, fake as unknown as GraphQLClient);
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await Promise.resolve();
    await Promise.resolve();
    fake.query.mockClear();

    eventBusManager.startBus(registered.id, fake as unknown as GraphQLClient);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E1',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: { __typename: 'ServerUpdatedEvent', name: 'stale' }
      });
    }
    await Promise.resolve();
    await Promise.resolve();

    expect(fake.query).toHaveBeenCalledTimes(2);
    expect(store.serverInfo.name).toBe('Fresh Name');
    expect(store.serverInfo.welcomeMessage).toBe('Fresh welcome');
    expect(store.serverInfo.description).toBe('Fresh description');
    expect(store.serverInfo.iconUrl).toBe('https://cdn/icon.webp');
    expect(store.serverInfo.bannerUrl).toBe('https://cdn/banner.webp');
    expect(store.serverInfo.motd).toBe('Fresh MOTD');
    expect(store.serverInfo.pushNotificationsEnabled).toBe(true);
    expect(store.serverInfo.livekitUrl).toBe('wss://livekit');
  });

  it('forwards RoomGroupsUpdatedEvent to the admin room layout store', async () => {
    const fake = new FakeGqlClient([
      {
        server: {
          pushNotificationsEnabled: false,
          vapidPublicKey: null,
          livekitUrl: null,
          videoProcessingEnabled: false,
          maxUploadSize: 25,
          maxVideoUploadSize: 25,
          messageEditWindowSeconds: 3600,
          profile: { motd: null }
        }
      },
      { server: { rooms: [] } },
      { server: { rooms: [] } },
      { server: { rooms: [], roomGroups: [] } },
      {
        server: {
          rooms: [{ id: 'r1', name: 'general', description: null, archived: false }],
          roomGroups: [{ id: 'g1', name: 'Lobby', rooms: [{ id: 'r1' }] }]
        }
      },
      { server: { rooms: [] } },
      { server: { rooms: [] } }
    ]);
    const store = new ServerStateStore(registered, fake as unknown as GraphQLClient);
    store.currentUser.user = { id: 'U1', login: 'alice', displayName: 'Alice' } as never;
    await Promise.resolve();
    await Promise.resolve();
    fake.query.mockClear();

    eventBusManager.startBus(registered.id, fake as unknown as GraphQLClient);
    flushSync();
    const bus = eventBusManager.getBus(registered.id);
    if (!bus) throw new Error('event bus did not start');

    for (const handler of bus.handlers) {
      handler({
        id: 'E2',
        createdAt: new Date().toISOString(),
        actorId: 'U1',
        actor: null,
        event: { __typename: 'RoomGroupsUpdatedEvent', changed: true }
      });
    }
    await Promise.resolve();
    await Promise.resolve();

    expect(fake.query).toHaveBeenCalledTimes(3);
    expect(store.adminRoomLayout.groups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        rooms: [{ id: 'r1', name: 'general', description: null, archived: false }]
      }
    ]);
  });
});
