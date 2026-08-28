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
  createdAt: string;
};

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  listEvents: vi.fn(),
  listEventTypes: vi.fn(),
  currentUrl: new URL('https://chat.example.test/chat/-/manage/server/event-log')
}));

let originalIntersectionObserver: typeof IntersectionObserver;
let observers: MockIntersectionObserver[] = [];

class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null;
  readonly rootMargin: string;
  readonly scrollMargin: string;
  readonly thresholds: ReadonlyArray<number> = [];
  private elements: Element[] = [];

  constructor(
    private readonly callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit
  ) {
    this.root = options?.root ?? null;
    this.rootMargin = options?.rootMargin ?? '0px';
    this.scrollMargin = options?.scrollMargin ?? '0px';
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

vi.mock('$app/state', () => ({
  page: {
    get url() {
      return mocks.currentUrl;
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn(),
  preloadData: vi.fn(),
  invalidate: vi.fn(),
  invalidateAll: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '').replace('[sequence]', params?.sequence ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: () => '-',
  segmentToServerId: (segment: string) => (segment === '-' ? 'origin' : null)
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {
      currentUser: { user: { settings: null } }
    },
    connection: {
      queryScope: 'event-log-test',
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/api-client/adminEventLog', async () => {
  const actual = await vi.importActual<typeof import('$lib/api-client/adminEventLog')>(
    '$lib/api-client/adminEventLog'
  );
  return {
    ...actual,
    createAdminEventLogAPI: () => ({
      listEvents: mocks.listEvents,
      listEventTypes: mocks.listEventTypes
    })
  };
});

function entry(sequence: string, eventType: string, createdAt = '2026-01-01T12:00:00Z'): Entry {
  return {
    sequence,
    subject: `evt.test.${sequence}`,
    aggregateType: 'test',
    aggregateId: sequence,
    eventType,
    eventId: `event-${sequence}`,
    actorId: `actor-${sequence}`,
    createdAt
  };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function pageResult(
  entries: Entry[],
  overrides: Partial<{
    totalCount: string;
    scannedCount: number;
    scanLimit: number;
    scanLimited: boolean;
    hasOlder: boolean;
    endCursor: string | null;
  }> = {}
) {
  return {
    entries,
    totalCount: overrides.totalCount ?? String(entries.length),
    scannedCount: overrides.scannedCount ?? entries.length,
    scanLimit: overrides.scanLimit ?? 50,
    scanLimited: overrides.scanLimited ?? false,
    hasOlder: overrides.hasOlder ?? false,
    endCursor: overrides.endCursor ?? null
  };
}

describe('server admin event log filters', () => {
  beforeEach(() => {
    originalIntersectionObserver = globalThis.IntersectionObserver;
    observers = [];
    globalThis.IntersectionObserver =
      MockIntersectionObserver as unknown as typeof IntersectionObserver;
    mocks.goto.mockReset();
    mocks.listEvents.mockReset();
    mocks.listEventTypes.mockReset();
    mocks.currentUrl = new URL('https://chat.example.test/chat/-/manage/server/event-log');
    mocks.listEvents.mockResolvedValue(
      pageResult([entry('102', 'UserJoinedRoomEvent'), entry('101', 'LoginSucceededEvent')], {
        totalCount: '2',
        hasOlder: true,
        endCursor: '101'
      })
    );
    mocks.listEventTypes.mockResolvedValue(['LoginSucceededEvent', 'UserJoinedRoomEvent']);
  });

  afterEach(() => {
    globalThis.IntersectionObserver = originalIntersectionObserver;
  });

  it('loads from URL filters and auto-loads older entries from the table sentinel', async () => {
    mocks.currentUrl = new URL(
      'https://chat.example.test/chat/-/manage/server/event-log?eventType=LoginSucceededEvent&actorId=user-1'
    );

    const { container } = render(EventLogPage);
    await settle();

    expect(mocks.listEventTypes).toHaveBeenCalledOnce();
    expect(mocks.listEvents).toHaveBeenCalledWith(
      {
        limit: 50,
        before: null,
        filter: {
          eventType: 'LoginSucceededEvent',
          actorId: 'user-1',
          createdAtFrom: '',
          createdAtTo: ''
        }
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(container.textContent).toContain('2 total events in stream');
    expect(container.textContent).toContain('UserJoinedRoomEvent');
    expect(container.textContent).toContain('LoginSucceededEvent');

    expect(observers).toHaveLength(1);
    observers[0].trigger(true);
    await settle();

    expect(mocks.listEvents).toHaveBeenCalledTimes(2);
  });

  it('requires an explicit action to continue after a capped filtered scan', async () => {
    mocks.listEvents.mockResolvedValue(
      pageResult([], { scanLimited: true, hasOlder: true, scanLimit: 5000, endCursor: '100' })
    );

    const { container } = render(EventLogPage);
    await settle();

    expect(container.textContent).toMatch(/may\s+have older matches outside that window/);
    expect(observers).toHaveLength(0);

    const scanOlder = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Scan older events')
    ) as HTMLButtonElement;
    scanOlder.click();
    await settle();

    expect(mocks.listEvents).toHaveBeenCalledTimes(2);
  });

  it('updates the URL when applying draft filters', async () => {
    const { container } = render(EventLogPage);
    await settle();

    const eventTypeInput = container.querySelector('#event-log-event-type') as HTMLInputElement;
    eventTypeInput.value = 'UserJoinedRoomEvent';
    eventTypeInput.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();

    const apply = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Apply')
    ) as HTMLButtonElement;
    apply.click();
    await settle();

    expect(mocks.goto).toHaveBeenCalledWith(
      '/chat/-/manage/server/event-log?eventType=UserJoinedRoomEvent',
      { keepFocus: true, noScroll: true }
    );
  });

  it('groups event rows by creation date', async () => {
    mocks.listEvents.mockResolvedValue(
      pageResult([
        entry('103', 'LoginSucceededEvent', '2026-01-02T12:00:00Z'),
        entry('102', 'UserJoinedRoomEvent', '2026-01-02T11:00:00Z'),
        entry('101', 'LoginSucceededEvent', '2026-01-01T12:00:00Z')
      ])
    );

    const { container } = render(EventLogPage);
    await settle();

    expect(container.textContent?.match(/Friday, January 2/g)).toHaveLength(1);
    expect(container.textContent?.match(/Thursday, January 1/g)).toHaveLength(1);
  });
});
