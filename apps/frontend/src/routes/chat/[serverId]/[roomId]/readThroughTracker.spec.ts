import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ReadThroughTracker, type TimelineReadPosition } from './readThroughTracker';

const DELAY = 1000;

function at(eventId: string, createdAtMs: number, latest = false): TimelineReadPosition {
  return { eventId, createdAtMs, latest };
}

describe('ReadThroughTracker', () => {
  let send: ReturnType<typeof vi.fn<(upToEventId: string | undefined) => void>>;
  let tracker: ReadThroughTracker;

  beforeEach(() => {
    vi.useFakeTimers();
    send = vi.fn<(upToEventId: string | undefined) => void>();
    tracker = new ReadThroughTracker(send, DELAY);
  });

  afterEach(() => {
    tracker.dispose();
    vi.useRealTimers();
  });

  it('ignores positions until the entry read is known', () => {
    tracker.observe(at('e2', 200));
    tracker.observe(at('e3', 300, true));
    vi.advanceTimersByTime(DELAY);

    expect(send).not.toHaveBeenCalled();
  });

  it('coalesces forward scrolling into one read after scrolling stops', () => {
    tracker.noteRead(100);

    tracker.observe(at('e2', 200));
    vi.advanceTimersByTime(DELAY / 2);
    tracker.observe(at('e3', 300));
    vi.advanceTimersByTime(DELAY - 1);
    expect(send).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(send).toHaveBeenCalledExactlyOnceWith('e3');
  });

  it('sends nothing for older or already read positions', () => {
    tracker.noteRead(300);

    tracker.observe(at('e1', 100));
    tracker.observe(at('e3', 300));
    vi.advanceTimersByTime(DELAY);

    expect(send).not.toHaveBeenCalled();
  });

  it('reads through the latest message at once when the viewer reaches it', () => {
    tracker.noteRead(100);
    tracker.observe(at('e2', 200));

    tracker.observe(at('e5', 500, true));
    expect(send).toHaveBeenCalledExactlyOnceWith(undefined);

    // The scheduled partial read is covered and dropped.
    vi.advanceTimersByTime(DELAY);
    expect(send).toHaveBeenCalledOnce();
  });

  it('does not read again when the viewer returns to an already read latest position', () => {
    tracker.noteRead(null);

    tracker.observe(at('e1', 100));
    tracker.observe(at('e5', 500, true));
    vi.advanceTimersByTime(DELAY);

    expect(send).not.toHaveBeenCalled();
  });

  it('includes a message that arrived while the viewer read older history', () => {
    tracker.noteRead(null);
    tracker.noteUnreadArrival(600);

    tracker.observe(at('e1', 100));
    vi.advanceTimersByTime(DELAY);
    expect(send).not.toHaveBeenCalled();

    tracker.observe(at('e6', 600, true));
    expect(send).toHaveBeenCalledExactlyOnceWith(undefined);
  });

  it('drops a scheduled read when the conversation changes', () => {
    tracker.noteRead(100);
    tracker.observe(at('e2', 200));

    tracker.reset();
    vi.advanceTimersByTime(DELAY);

    expect(send).not.toHaveBeenCalled();
  });
});
