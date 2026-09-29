import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { PresenceChangedEvent, UserTypingEvent } from '@chatto/api-types/realtime/v1/events_pb';
import {
  EventBus,
  RealtimeProjectionUpdate,
  type ProjectionHandler
} from '@chatto/client/realtime/eventBus';

const serverScope = $state({ serverId: 'origin' });

const { mocks } = vi.hoisted(() => ({
  mocks: {
    getBus: vi.fn(),
    useServerScope: vi.fn()
  }
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  eventBusManager: { getBus: mocks.getBus }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: mocks.useServerScope
}));

import { useProjectionEvent, useTypingEvent } from './useEvent.svelte';

let buses: Map<string, EventBus>;

/** Create a bus that the mocked manager returns for this server. */
function registerBus(serverId: string): EventBus {
  const bus = new EventBus(serverId);
  bus.setReducer(vi.fn());
  buses.set(serverId, bus);
  return bus;
}

beforeEach(() => {
  serverScope.serverId = 'origin';
  buses = new Map();
  mocks.useServerScope.mockReset();
  mocks.useServerScope.mockImplementation(() => serverScope);
  mocks.getBus.mockReset();
  mocks.getBus.mockImplementation((serverId: string) => buses.get(serverId));
});

describe('useProjectionEvent', () => {
  it('moves the subscription when the route server scope changes', () => {
    const origin = registerBus('origin');
    const remote = registerBus('remote');
    const handler = vi.fn<ProjectionHandler>();

    const dispose = $effect.root(() => {
      useProjectionEvent(handler);
    });
    flushSync();

    expect(mocks.getBus).toHaveBeenCalledWith('origin');
    expect(origin.listenerCount).toBe(1);
    expect(remote.listenerCount).toBe(0);

    serverScope.serverId = 'remote';
    flushSync();

    expect(origin.listenerCount).toBe(0);
    expect(remote.listenerCount).toBe(1);
    const update = new RealtimeProjectionUpdate({ id: 'evt-1' });
    remote.publish(update);
    expect(handler).toHaveBeenCalledExactlyOnceWith(update);

    dispose();
    expect(remote.listenerCount).toBe(0);
  });

  it('uses an explicit origin selector without reading route context', () => {
    const origin = registerBus('origin');

    const dispose = $effect.root(() => {
      useProjectionEvent(vi.fn<ProjectionHandler>(), () => 'origin');
    });
    flushSync();

    expect(mocks.useServerScope).not.toHaveBeenCalled();
    expect(mocks.getBus).toHaveBeenCalledWith('origin');
    expect(origin.listenerCount).toBe(1);
    dispose();
  });

  it('does nothing when the selected server has no bus', () => {
    const dispose = $effect.root(() => {
      useProjectionEvent(vi.fn<ProjectionHandler>(), () => 'unknown');
    });
    flushSync();

    expect(mocks.getBus).toHaveBeenCalledWith('unknown');
    expect(() => dispose()).not.toThrow();
  });
});

describe('useTypingEvent', () => {
  it('receives typing signals with their room and thread', () => {
    const origin = registerBus('origin');
    const handler = vi.fn();
    const dispose = $effect.root(() => {
      useTypingEvent(handler);
    });
    flushSync();

    const typing = (actorId: string, threadRootEventId?: string) =>
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          actorId,
          event: {
            case: 'userTyping',
            value: new UserTypingEvent({ roomId: 'R1', threadRootEventId })
          }
        })
      });
    origin.publish(typing('U2'));
    origin.publish(typing('U3', 'ROOT'));
    origin.publish(typing(''));
    origin.publish(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          actorId: 'U4',
          event: { case: 'presenceChanged', value: new PresenceChangedEvent() }
        })
      })
    );

    expect(handler.mock.calls).toEqual([
      [{ userId: 'U2', roomId: 'R1', threadRootEventId: null }],
      [{ userId: 'U3', roomId: 'R1', threadRootEventId: 'ROOT' }]
    ]);
    dispose();
    expect(origin.listenerCount).toBe(0);
  });
});
