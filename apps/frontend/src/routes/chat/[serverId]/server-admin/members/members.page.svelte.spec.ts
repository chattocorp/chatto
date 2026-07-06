import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import MembersPage from './+page.svelte';

type Member = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl: string | null;
  roles: string[];
  createdAt: string;
};

const mocks = vi.hoisted(() => ({
  listMembers: vi.fn(),
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

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    connectBaseUrl: 'http://localhost/api/connect',
    bearerToken: null
  })
}));

vi.mock('$lib/api-client/adminUsers', async () => {
  const actual = await vi.importActual<typeof import('$lib/api-client/adminUsers')>('$lib/api-client/adminUsers');
  return {
    ...actual,
    createAdminUserManagementAPI: () => ({
      listMembers: mocks.listMembers
    })
  };
});

function member(index: number, prefix = 'member'): Member {
  return {
    id: `${prefix}-${index}`,
    login: `${prefix}${index}`,
    displayName: `${prefix} ${index}`,
    avatarUrl: null,
    roles: ['everyone'],
    createdAt: '2026-01-01T12:00:00Z'
  };
}

function result(users: Member[], totalCount = users.length, hasMore = false) {
  return {
    roles: [{ name: 'admin', displayName: 'Admin' }],
    users,
    totalCount,
    hasMore
  };
}

function queueResults(...results: ReturnType<typeof result>[]) {
  mocks.listMembers.mockImplementation(() => {
    return Promise.resolve(results.shift());
  });
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('server admin members pagination', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    originalIntersectionObserver = globalThis.IntersectionObserver;
    observers = [];
    globalThis.IntersectionObserver =
      MockIntersectionObserver as unknown as typeof IntersectionObserver;
    mocks.listMembers.mockReset();
    mocks.goto.mockReset();
  });

  afterEach(() => {
    globalThis.IntersectionObserver = originalIntersectionObserver;
    vi.useRealTimers();
  });

  it('loads the first offset page on mount and the next page when the table end intersects', async () => {
    queueResults(
      result(
        Array.from({ length: 20 }, (_, i) => member(i)),
        21,
        true
      ),
      result([member(20)], 21, false)
    );

    const { container } = render(MembersPage);
    await settle();

    expect(mocks.listMembers).toHaveBeenNthCalledWith(1, {
      search: null,
      limit: 20,
      offset: 0
    });
    expect(container.textContent).toContain('Showing 20 of 21 member(s)');

    expect(observers).toHaveLength(1);
    observers[0].trigger(true);
    await settle();

    expect(mocks.listMembers).toHaveBeenNthCalledWith(2, {
      search: null,
      limit: 20,
      offset: 20
    });
    expect(container.textContent).toContain('@member20');
    expect(container.textContent).toContain('Showing 21 of 21 member(s)');
  });

  it('searches from offset zero and hides load-more when the filtered page is complete', async () => {
    queueResults(
      result([member(0, 'unrelated')], 42, true),
      result([member(0, 'target')], 1, false)
    );

    const { container } = render(MembersPage);
    await settle();

    const input = container.querySelector('input') as HTMLInputElement;
    input.value = ' target ';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await vi.advanceTimersByTimeAsync(300);
    await settle();

    expect(mocks.listMembers).toHaveBeenNthCalledWith(2, {
      search: 'target',
      limit: 20,
      offset: 0
    });
    expect(container.textContent).toContain('@target0');
    expect(container.textContent).not.toContain('@unrelated0');
  });

  it('renders the members body as a scroll region', async () => {
    queueResults(result([], 0, false));

    const { container } = render(MembersPage);
    await settle();

    expect(container.querySelector('.min-h-0.flex-1.overflow-y-auto')).toBeTruthy();
  });
});
