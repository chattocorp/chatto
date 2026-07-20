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
  loadFirstPage: vi.fn(),
  loadMore: vi.fn(),
  loadEventTypes: vi.fn(),
  currentUrl: new URL('https://chat.example.test/chat/-/manage/server/event-log'),
  eventLog: {
    entries: [] as Entry[],
    totalCount: '0',
    scannedCount: 0,
    scanLimit: 50,
    scanLimited: false,
    hasOlder: false,
    endCursor: null as string | null,
    loading: false,
    loadingMore: false,
    error: null as string | null,
    compatibilityMessage: null as string | null,
    activeFilter: {
      eventType: '',
      actorId: '',
      createdAtFrom: '',
      createdAtTo: ''
    },
    eventTypes: ['LoginSucceededEvent', 'UserJoinedRoomEvent'],
    eventTypesLoading: false,
    eventTypesUnsupported: false,
    get hasActiveFilter() {
      return Boolean(
        this.activeFilter.eventType ||
        this.activeFilter.actorId ||
        this.activeFilter.createdAtFrom ||
        this.activeFilter.createdAtTo
      );
    }
  }
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

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/userSettings.svelte', () => ({
  getUserSettings: () => ({
    effectiveTimezone: undefined,
    effectiveHour12: undefined
  })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      adminEventLog: {
        ...mocks.eventLog,
        loadFirstPage: mocks.loadFirstPage,
        loadMore: mocks.loadMore,
        loadEventTypes: mocks.loadEventTypes
      }
    })
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    client: {
      query: vi.fn(() => ({
        toPromise: vi.fn().mockResolvedValue({
          data: {
            server: {
              members: {
                users: []
              }
            }
          },
          error: null
        })
      }))
    }
  })
}));

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
  flushSync();
}

describe('server admin event log filters', () => {
  beforeEach(() => {
    originalIntersectionObserver = globalThis.IntersectionObserver;
    observers = [];
    globalThis.IntersectionObserver =
      MockIntersectionObserver as unknown as typeof IntersectionObserver;
    mocks.goto.mockReset();
    mocks.loadFirstPage.mockReset();
    mocks.loadMore.mockReset();
    mocks.loadEventTypes.mockReset();
    mocks.currentUrl = new URL('https://chat.example.test/chat/-/manage/server/event-log');
    mocks.eventLog.entries = [
      entry('102', 'UserJoinedRoomEvent'),
      entry('101', 'LoginSucceededEvent')
    ];
    mocks.eventLog.totalCount = '2';
    mocks.eventLog.scannedCount = 2;
    mocks.eventLog.scanLimit = 50;
    mocks.eventLog.scanLimited = false;
    mocks.eventLog.hasOlder = true;
    mocks.eventLog.loading = false;
    mocks.eventLog.loadingMore = false;
    mocks.eventLog.error = null;
    mocks.eventLog.compatibilityMessage = null;
    mocks.eventLog.activeFilter = {
      eventType: '',
      actorId: '',
      createdAtFrom: '',
      createdAtTo: ''
    };
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

    expect(mocks.loadEventTypes).toHaveBeenCalledOnce();
    expect(mocks.loadFirstPage).toHaveBeenCalledWith({
      eventType: 'LoginSucceededEvent',
      actorId: 'user-1',
      createdAtFrom: '',
      createdAtTo: ''
    });
    expect(container.textContent).toContain('2 total events in stream');
    expect(container.textContent).toContain('UserJoinedRoomEvent');
    expect(container.textContent).toContain('LoginSucceededEvent');

    expect(observers).toHaveLength(1);
    observers[0].trigger(true);
    await settle();

    expect(mocks.loadMore).toHaveBeenCalledOnce();
  });

  it('requires an explicit action to continue after a capped filtered scan', async () => {
    mocks.eventLog.scanLimited = true;
    mocks.eventLog.hasOlder = true;
    mocks.eventLog.scanLimit = 5000;

    const { container } = render(EventLogPage);
    await settle();

    expect(container.textContent).toMatch(/may\s+have older matches outside that window/);
    expect(observers).toHaveLength(0);

    const scanOlder = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Scan older events')
    ) as HTMLButtonElement;
    scanOlder.click();
    await settle();

    expect(mocks.loadMore).toHaveBeenCalledOnce();
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
    mocks.eventLog.entries = [
      entry('103', 'LoginSucceededEvent', '2026-01-02T12:00:00Z'),
      entry('102', 'UserJoinedRoomEvent', '2026-01-02T11:00:00Z'),
      entry('101', 'LoginSucceededEvent', '2026-01-01T12:00:00Z')
    ];

    const { container } = render(EventLogPage);
    await settle();

    expect(container.textContent?.match(/Friday, January 2/g)).toHaveLength(1);
    expect(container.textContent?.match(/Thursday, January 1/g)).toHaveLength(1);
  });
});
