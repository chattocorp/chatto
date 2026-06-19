import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import EventLogPage from './+page.svelte';

type Entry = {
  sequence: string;
  subject: string;
  aggregateType: string;
  aggregateId: string;
  eventType: string;
  eventId: string;
  actorId: string;
  createdAt: { toDate(): Date };
};

const mocks = vi.hoisted(() => ({
  listAdminEventLog: vi.fn(),
  goto: vi.fn()
}));

let originalIntersectionObserver: typeof IntersectionObserver;
let observers: MockIntersectionObserver[] = [];

class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null;
  readonly rootMargin: string;
  readonly thresholds: ReadonlyArray<number> = [];
  private elements: Element[] = [];

  constructor(
    private readonly callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit
  ) {
    this.root = options?.root ?? null;
    this.rootMargin = options?.rootMargin ?? '0px';
    observers.push(this);
  }

  observe = (target: Element) => {
    this.elements.push(target);
  };

  unobserve = (target: Element) => {
    this.elements = this.elements.filter((element) => element !== target);
  };

  disconnect = () => {
    this.elements = [];
  };

  takeRecords = () => [];

  trigger(isIntersecting: boolean) {
    const target = this.elements[0] ?? document.createElement('tr');
    this.callback(
      [
        {
          boundingClientRect: target.getBoundingClientRect(),
          intersectionRatio: isIntersecting ? 1 : 0,
          intersectionRect: target.getBoundingClientRect(),
          isIntersecting,
          rootBounds: null,
          target,
          time: performance.now()
        }
      ],
      this
    );
  }
}

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn(),
  preloadData: vi.fn(),
  invalidate: vi.fn(),
  invalidateAll: vi.fn()
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/userSettings.svelte', () => ({
  getUserSettings: () => ({
    effectiveTimezone: undefined,
    effectiveHour12: undefined
  })
}));

vi.mock('$lib/wire/activeServerClient', () => ({
  withActiveServerWireClient: vi.fn((callback: (client: unknown) => unknown) =>
    callback({
      listAdminEventLog: mocks.listAdminEventLog
    })
  )
}));

function entry(sequence: string, eventType: string): Entry {
  return {
    sequence,
    subject: `evt.test.${sequence}`,
    aggregateType: 'test',
    aggregateId: sequence,
    eventType,
    eventId: `event-${sequence}`,
    actorId: `actor-${sequence}`,
    createdAt: { toDate: () => new Date('2026-01-01T12:00:00Z') }
  };
}

function result(
  entries: Entry[],
  totalCount = entries.length,
  hasOlder = false,
  endCursor?: string
) {
  return {
    entries,
    totalCount,
    hasOlder,
    endCursor: endCursor ?? entries.at(-1)?.sequence ?? ''
  };
}

function queueResults(...results: Array<ReturnType<typeof result> | Error>) {
  mocks.listAdminEventLog.mockImplementation(() => {
    const data = results.shift();
    if (data instanceof Error) return Promise.reject(data);
    return Promise.resolve(data);
  });
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('server admin event log pagination', () => {
  beforeEach(() => {
    originalIntersectionObserver = globalThis.IntersectionObserver;
    observers = [];
    globalThis.IntersectionObserver =
      MockIntersectionObserver as unknown as typeof IntersectionObserver;
    mocks.listAdminEventLog.mockReset();
    mocks.goto.mockReset();
  });

  afterEach(() => {
    globalThis.IntersectionObserver = originalIntersectionObserver;
  });

  it('loads the first cursor page and auto-loads older entries from the table sentinel', async () => {
    queueResults(
      result([entry('102', 'user.created'), entry('101', 'room.created')], 3, true, '101'),
      result([entry('101', 'room.created'), entry('100', 'auth.login')], 3, false, '100')
    );

    const { container } = render(EventLogPage);
    await settle();

    expect(mocks.listAdminEventLog).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({
        limit: 50,
        before: ''
      })
    );
    expect(container.textContent).toContain('3 total events in stream');
    expect(container.textContent).toContain('user.created');
    expect(container.textContent).toContain('room.created');

    expect(observers).toHaveLength(1);
    observers[0].trigger(true);
    await settle();

    expect(mocks.listAdminEventLog).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        limit: 50,
        before: '101'
      })
    );
    expect(container.textContent).toContain('auth.login');
    expect(container.textContent?.match(/room.created/g)).toHaveLength(1);
  });

  it('renders the audit permission error when admin data is unavailable', async () => {
    queueResults(new Error('Event log unavailable (audit permission required)'));

    const { container } = render(EventLogPage);
    await settle();

    expect(container.textContent).toContain('Event log unavailable (audit permission required)');
    expect(observers).toHaveLength(0);
  });
});
