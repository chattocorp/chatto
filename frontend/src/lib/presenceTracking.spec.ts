import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import type { Client } from '@urql/svelte';
import { PresenceStatus, PresenceStatusInput } from './gql/graphql';
import { initPresenceTracking } from './presenceTracking';

type MockClient = Pick<Client, 'mutation'>;
type PresenceMutation = (
  document: unknown,
  variables: { input: { status: PresenceStatusInput } }
) => { toPromise: () => Promise<unknown> };
type PresenceStatusHandler = (status: PresenceStatus) => void;

let documentTarget: EventTarget;
let windowTarget: EventTarget;
let visibilityState: DocumentVisibilityState;
let cleanup: (() => void) | null;
let mutation: Mock<PresenceMutation>;
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
  mutation = vi.fn<PresenceMutation>(() => ({
    toPromise: () => Promise.resolve({ data: { updateMyPresence: true } })
  }));
  onStatusChange = vi.fn<PresenceStatusHandler>();
  const client = { mutation } as unknown as MockClient;
  cleanup = initPresenceTracking(() => [client as Client], onStatusChange);
}

function sentStatuses(): PresenceStatusInput[] {
  return mutation.mock.calls.map((call) => call[1].input.status);
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

    expect(sentStatuses()).not.toContain(PresenceStatusInput.Away);
    expect(onStatusChange).not.toHaveBeenCalledWith(PresenceStatus.Away);
  });

  it.each(['wheel', 'scroll', 'keydown', 'pointerdown'] as const)(
    'resets the idle timer on %s',
    (eventName) => {
      startTracking();

      vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);
      dispatchDocumentEvent(eventName);
      vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);

      expect(sentStatuses()).not.toContain(PresenceStatusInput.Away);
      expect(onStatusChange).not.toHaveBeenCalledWith(PresenceStatus.Away);
    }
  );

  it('returns online when broad activity resumes after idle', () => {
    startTracking();

    vi.advanceTimersByTime(5 * 60 * 1000);
    expect(sentStatuses()).toEqual([PresenceStatusInput.Away]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Away);

    dispatchDocumentEvent('pointermove');

    expect(sentStatuses()).toEqual([PresenceStatusInput.Away, PresenceStatusInput.Online]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Online);
  });

  it('reports away after the hidden delay and returns online when visible again', () => {
    startTracking();

    setVisibility('hidden');
    vi.advanceTimersByTime(9_999);
    expect(sentStatuses()).toEqual([]);

    vi.advanceTimersByTime(1);
    expect(sentStatuses()).toEqual([PresenceStatusInput.Away]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Away);

    setVisibility('visible');

    expect(sentStatuses()).toEqual([PresenceStatusInput.Away, PresenceStatusInput.Online]);
    expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Online);
  });

  it('throttles noisy activity while active without delaying return from idle', () => {
    startTracking();

    for (let i = 0; i < 20; i++) {
      dispatchDocumentEvent('pointermove');
      vi.advanceTimersByTime(50);
    }

    expect(sentStatuses()).toEqual([]);

    vi.advanceTimersByTime(5 * 60 * 1000);
    expect(sentStatuses()).toEqual([PresenceStatusInput.Away]);

    dispatchDocumentEvent('wheel');

    expect(sentStatuses()).toEqual([PresenceStatusInput.Away, PresenceStatusInput.Online]);
  });

  it('returns online on window focus after idle', () => {
    startTracking();

    vi.advanceTimersByTime(5 * 60 * 1000);
    dispatchWindowEvent('focus');

    expect(sentStatuses()).toEqual([PresenceStatusInput.Away, PresenceStatusInput.Online]);
  });
});
