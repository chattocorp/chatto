import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import MembersPage from './+page.svelte';

type Member = {
  user: {
    id: string;
    login: string;
    displayName: string;
    createdAt: { toDate(): Date };
  };
  avatarUrl: string;
  roles: string[];
};

const mocks = vi.hoisted(() => ({
  listAdminMembers: vi.fn(),
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
  withActiveServerWireClient: (callback: (client: { listAdminMembers: typeof mocks.listAdminMembers }) => unknown) =>
    callback({ listAdminMembers: mocks.listAdminMembers })
}));

function member(index: number, prefix = 'member'): Member {
  return {
    user: {
      id: `${prefix}-${index}`,
      login: `${prefix}${index}`,
      displayName: `${prefix} ${index}`,
      createdAt: { toDate: () => new Date('2026-01-01T12:00:00Z') }
    },
    avatarUrl: '',
    roles: ['admin']
  };
}

function result(users: Member[], totalCount = users.length, hasMore = false) {
  return {
    roles: [{ name: 'admin', displayName: 'Admin' }],
    members: users,
    totalCount,
    hasMore
  };
}

function queueResults(...results: ReturnType<typeof result>[]) {
  mocks.listAdminMembers.mockImplementation(() => {
    const data = results.shift();
    return Promise.resolve(data);
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
    mocks.listAdminMembers.mockReset();
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

    expect(mocks.listAdminMembers).toHaveBeenNthCalledWith(1, {
      search: '',
      limit: 20,
      offset: 0
    });
    expect(container.textContent).toContain('Showing 20 of 21 member(s)');

    expect(observers).toHaveLength(1);
    observers[0].trigger(true);
    await settle();

    expect(mocks.listAdminMembers).toHaveBeenNthCalledWith(2, {
      search: '',
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

    expect(mocks.listAdminMembers).toHaveBeenNthCalledWith(2, {
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
