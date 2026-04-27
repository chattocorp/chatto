import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync, tick } from 'svelte';
import type { Mock } from 'vitest';
import { CombinedError } from '@urql/svelte';
import Harness from './useRoomDataHarness.svelte';

const queryMock = vi.hoisted(() => vi.fn());

vi.mock('$lib/state/instance/connection.svelte', () => ({
  useConnection: () => () => ({
    client: { query: queryMock },
    isConnected: true,
    showConnectionLostBanner: false
  })
}));

vi.mock('./useReconnectCallback.svelte', () => ({
  useReconnectTrigger: () => ({
    get count() {
      return 0;
    }
  }),
  useReconnectCallback: () => {}
}));

function networkErrorResponse() {
  return {
    data: undefined,
    error: new CombinedError({ networkError: new Error('fetch failed') })
  };
}

function successResponse(roomId = 'R1', spaceId = 'S1') {
  return {
    data: {
      room: {
        id: roomId,
        name: 'general',
        viewerCanPostMessage: true,
        viewerCanPostInThread: true,
        viewerCanReply: true,
        viewerCanReplyInThread: true,
        viewerCanReact: true,
        viewerCanEditOwnMessage: true,
        viewerCanEditAnyMessage: false,
        viewerCanDeleteOwnMessage: true,
        viewerCanDeleteAnyMessage: false,
        viewerCanEchoMessage: false,
        members: []
      },
      space: { id: spaceId, name: 'Test Space', viewerCanManageRooms: false }
    },
    error: undefined
  };
}

function notFoundResponse() {
  return { data: { room: null, space: null }, error: undefined };
}

function queueResponses(...responses: unknown[]): void {
  queryMock.mockReset();
  for (const resp of responses) {
    queryMock.mockReturnValueOnce({ toPromise: () => Promise.resolve(resp) });
  }
}

type Snapshot = {
  roomData: unknown;
  isRoomLoading: boolean;
};

function renderHarness(spaceId = 'S1', roomId = 'R1') {
  const snapshots: Snapshot[] = [];
  const result = render(Harness, {
    props: {
      spaceId,
      roomId,
      onSnapshot: (s: Snapshot) => snapshots.push(s)
    }
  });
  return { snapshots, ...result };
}

async function flushMicrotasks() {
  // urql `toPromise()` resolves through microtasks; we need a few ticks for
  // the chained `.then`s plus Svelte's effect flush.
  for (let i = 0; i < 5; i++) {
    await tick();
  }
  flushSync();
}

describe('useRoomData', () => {
  let warnSpy: Mock;
  let errorSpy: Mock;

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {}) as unknown as Mock;
    errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {}) as unknown as Mock;
    queryMock.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    warnSpy.mockRestore();
    errorSpy.mockRestore();
  });

  it('retries on networkError during initial load and resolves on success', async () => {
    queueResponses(networkErrorResponse(), networkErrorResponse(), successResponse());
    const { snapshots } = renderHarness();

    await flushMicrotasks();
    expect(queryMock).toHaveBeenCalledTimes(1);
    expect(snapshots.at(-1)?.roomData).toBeUndefined();
    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining('retrying in 1000ms'),
      expect.anything()
    );

    await vi.advanceTimersByTimeAsync(1000);
    await flushMicrotasks();
    expect(queryMock).toHaveBeenCalledTimes(2);
    expect(snapshots.at(-1)?.roomData).toBeUndefined();

    await vi.advanceTimersByTimeAsync(3000);
    await flushMicrotasks();
    expect(queryMock).toHaveBeenCalledTimes(3);
    expect(snapshots.at(-1)?.roomData).toMatchObject({ room: { id: 'R1', name: 'general' } });
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it('exhausts retries and leaves roomData undefined (does not flip to null)', async () => {
    queueResponses(
      networkErrorResponse(),
      networkErrorResponse(),
      networkErrorResponse(),
      networkErrorResponse()
    );
    const { snapshots } = renderHarness();

    await flushMicrotasks();
    await vi.advanceTimersByTimeAsync(1000);
    await flushMicrotasks();
    await vi.advanceTimersByTimeAsync(3000);
    await flushMicrotasks();
    await vi.advanceTimersByTimeAsync(8000);
    await flushMicrotasks();

    expect(queryMock).toHaveBeenCalledTimes(4);
    expect(snapshots.at(-1)?.roomData).toBeUndefined();
    expect(errorSpy).toHaveBeenCalledWith(
      expect.stringContaining('retries exhausted'),
      expect.anything()
    );
  });

  it('flips to null on a clean not-found response (no retry)', async () => {
    queueResponses(notFoundResponse());
    const { snapshots } = renderHarness();

    await flushMicrotasks();
    expect(queryMock).toHaveBeenCalledTimes(1);
    expect(snapshots.at(-1)?.roomData).toBeNull();
    expect(warnSpy).not.toHaveBeenCalled();
  });
});
