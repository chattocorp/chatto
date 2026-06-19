import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import type { UpdateMyPresenceRequest } from '$lib/pb/chatto/api/v1/chat_pb';
import { UserPresenceStatus } from '$lib/pb/chatto/core/v1/models_pb';
import {
  initPresenceTracking,
  PresenceTrackerStatus,
  type PresenceClient
} from './presenceTracking';

type PresenceUpdate = (request: UpdateMyPresenceRequest) => Promise<unknown>;
type PresenceStatusHandler = (status: PresenceTrackerStatus) => void;

let documentTarget: EventTarget;
let windowTarget: EventTarget;
let visibilityState: DocumentVisibilityState;
let cleanup: (() => void) | null;
let updateMyPresence: Mock<PresenceUpdate>;
let onStatusChange: Mock<PresenceStatusHandler>;

function dispatchDocumentEvent(type: string) {
  documentTarget.dispatchEvent(new Event(type));
}

function dispatchWindowEvent(type: string) {
  windowTarget.dispatchEvent(new Event(type));
}

function setVisibility(next: DocumentVisibilityState) {
  visibilityState = next;
  dispatchDocumentEvent('visibilitychange');
}

function startTracking() {
  updateMyPresence = vi.fn<PresenceUpdate>(() => Promise.resolve({ updated: true }));
  onStatusChange = vi.fn<PresenceStatusHandler>();
  const client = { updateMyPresence } satisfies PresenceClient;
  cleanup = initPresenceTracking(() => [client], onStatusChange);
}

function sentStatuses(): UserPresenceStatus[] {
  return updateMyPresence.mock.calls.map((call) => call[0].status);
}

describe('initPresenceTracking', () => {
  beforeEach(() => {
    vi.useFakeTimers({ now: 0 });
    documentTarget = new EventTarget();
    windowTarget = new EventTarget();
    visibilityState = 'visible';
    cleanup = null;

    vi.stubGlobal('document', {
      addEventListener: documentTarget.addEventListener.bind(documentTarget),
      removeEventListener: documentTarget.removeEventListener.bind(documentTarget),
      dispatchEvent: documentTarget.dispatchEvent.bind(documentTarget),
      get visibilityState() {
        return visibilityState;
      }
    });
    vi.stubGlobal('window', {
      addEventListener: windowTarget.addEventListener.bind(windowTarget),
      removeEventListener: windowTarget.removeEventListener.bind(windowTarget),
      dispatchEvent: windowTarget.dispatchEvent.bind(windowTarget)
    });
  });

  afterEach(() => {
    cleanup?.();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('does not report away while pointer movement continues before the idle timeout', () => {
    startTracking();

    vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);
    dispatchDocumentEvent('pointermove');
    vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);

    expect(sentStatuses()).not.toContain(UserPresenceStatus.AWAY);
    expect(onStatusChange).not.toHaveBeenCalledWith(PresenceTrackerStatus.Away);
  });

  it.each(['wheel', 'scroll', 'keydown', 'pointerdown'] as const)(
    'resets the idle timer on %s',
    (eventName) => {
      startTracking();

      vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);
      dispatchDocumentEvent(eventName);
      vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);

      expect(sentStatuses()).not.toContain(UserPresenceStatus.AWAY);
      expect(onStatusChange).not.toHaveBeenCalledWith(PresenceTrackerStatus.Away);
    }
  );

  it('returns online when broad activity resumes after idle', () => {
    startTracking();

    vi.advanceTimersByTime(5 * 60 * 1000);
    expect(sentStatuses()).toEqual([UserPresenceStatus.AWAY]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceTrackerStatus.Away);

    dispatchDocumentEvent('pointermove');

    expect(sentStatuses()).toEqual([UserPresenceStatus.AWAY, UserPresenceStatus.ONLINE]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceTrackerStatus.Online);
  });

  it('reports away after the hidden delay and returns online when visible again', () => {
    startTracking();

    setVisibility('hidden');
    vi.advanceTimersByTime(9_999);
    expect(sentStatuses()).toEqual([]);

    vi.advanceTimersByTime(1);
    expect(sentStatuses()).toEqual([UserPresenceStatus.AWAY]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceTrackerStatus.Away);

    setVisibility('visible');

    expect(sentStatuses()).toEqual([UserPresenceStatus.AWAY, UserPresenceStatus.ONLINE]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceTrackerStatus.Online);
  });

  it('throttles noisy activity while active without delaying return from idle', () => {
    startTracking();

    for (let i = 0; i < 20; i++) {
      dispatchDocumentEvent('pointermove');
      vi.advanceTimersByTime(50);
    }

    expect(sentStatuses()).toEqual([]);

    vi.advanceTimersByTime(5 * 60 * 1000);
    expect(sentStatuses()).toEqual([UserPresenceStatus.AWAY]);

    dispatchDocumentEvent('wheel');

    expect(sentStatuses()).toEqual([UserPresenceStatus.AWAY, UserPresenceStatus.ONLINE]);
  });

  it('returns online on window focus after idle', () => {
    startTracking();

    vi.advanceTimersByTime(5 * 60 * 1000);
    dispatchWindowEvent('focus');

    expect(sentStatuses()).toEqual([UserPresenceStatus.AWAY, UserPresenceStatus.ONLINE]);
  });
});
