import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import {
  NotificationAttentionLevel,
  NotificationSignalKind,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import { nextUnreadSource } from '$lib/notifications/nextUnreadSource';
import { quickSwitcher } from '$lib/state/globals.svelte';
import { flushSync } from 'svelte';
import Harness from './GlobalKeyboardShortcutsTestHarness.svelte';

const mocks = vi.hoisted(() => ({
  goto: vi.fn().mockResolvedValue(undefined),
  info: vi.fn(),
  error: vi.fn(),
  stores: new Map<string, ReturnType<typeof makeStore>>()
}));
vi.mock('$app/environment', () => ({
  browser: true,
  building: false,
  version: 'test',
  dev: false
}));
vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn(),
  afterNavigate: vi.fn(),
  beforeNavigate: vi.fn()
}));
vi.mock('$lib/ui/toast', () => ({ toast: { info: mocks.info } }));
vi.mock('$app/paths', () => ({
  base: '',
  assets: '',
  resolve: (path: string, params?: Record<string, string>) =>
    Object.entries(params ?? {}).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));
vi.mock('$lib/utils/errorMessage', () => ({ toastError: mocks.error }));
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    isOriginServer: (id: string) => id === 'origin',
    getServer: (id: string) => ({ id, url: `https://${id}.example.test` }),
    get servers() {
      return [...mocks.stores.keys()].map((id) => ({ id }));
    },
    getStore: (id: string) => mocks.stores.get(id),
    tryGetStore: (id: string) => mocks.stores.get(id)
  }
}));

function occurrence(
  id: string,
  roomId = id,
  extra: Partial<NotificationOccurrenceItem> = {}
): NotificationOccurrenceItem {
  return {
    id,
    room: { id: roomId, name: roomId },
    createdAt: '2026-10-27T12:00:00Z',
    actor: null,
    eventId: `event-${id}`,
    threadRootId: null,
    signalKind: NotificationSignalKind.DIRECT_MENTION,
    targetSupported: true,
    attentionLevel: NotificationAttentionLevel.IMPORTANT,
    unread: true,
    reactionEmoji: null,
    ...extra
  };
}

function makeStore(rows: NotificationOccurrenceItem[]) {
  return {
    isAuthenticated: true,
    projection: { rooms: new Map() },
    accountId: 'viewer',
    onSessionEnded: vi.fn((_callback: () => void) => vi.fn()),
    onReset: vi.fn((_callback: (reset: { privacy: boolean }) => void) => vi.fn()),
    notifications: {
      hasLoaded: true,
      fetchPage: vi.fn(),
      fetchAllPages: vi.fn().mockResolvedValue(undefined),
      markOccurrenceRead: vi.fn().mockResolvedValue(undefined)
    },
    attention: { occurrences: rows },
    navigation: { roomGroups: [{ roomIds: ['a', 'b'] }], rooms: [], isInitialLoading: false },
    pendingHighlights: { set: vi.fn() }
  };
}

function press(extra: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', {
    key: 'ArrowDown',
    altKey: true,
    shiftKey: true,
    bubbles: true,
    cancelable: true,
    ...extra
  });
  (document.activeElement ?? window).dispatchEvent(event);
  return event;
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.stores.clear();
  quickSwitcher.close();
});

describe('production reload shortcuts', () => {
  it.each([
    { key: 'r', metaKey: true },
    { key: 'r', ctrlKey: true },
    { key: 'R', metaKey: true, shiftKey: true },
    { key: 'R', ctrlKey: true, shiftKey: true },
    { key: 'F5' },
    { key: 'F5', ctrlKey: true },
    { key: 'F5', shiftKey: true }
  ])('blocks reload from a focused editor: %j', (shortcut) => {
    const { container } = render(Harness);
    container.querySelector('textarea')!.focus();
    expect(press({ altKey: false, shiftKey: false, ...shortcut }).defaultPrevented).toBe(true);
  });

  it('keeps ordinary typing available', () => {
    render(Harness);
    expect(press({ key: 'r', altKey: false, shiftKey: false }).defaultPrevented).toBe(false);
  });
});

describe('next unread source', () => {
  it('prioritises Important across servers, then room order and newest activity in a source', () => {
    const ambient = occurrence('ambient', 'a', {
      attentionLevel: NotificationAttentionLevel.AMBIENT
    });
    const old = occurrence('old', 'b', { createdAt: '2026-10-26T12:00:00Z' });
    const newest = occurrence('new', 'b');
    const servers = [
      { serverId: 'first', roomIds: ['a', 'b'], occurrences: [ambient] },
      { serverId: 'second', roomIds: ['b', 'a'], occurrences: [occurrence('a'), old, newest] }
    ];
    expect(nextUnreadSource(servers)?.occurrence.id).toBe('new');
    servers[1]!.occurrences = [];
    expect(nextUnreadSource(servers)?.occurrence.id).toBe('ambient');
  });

  it('excludes read and unsupported occurrences and keeps separate threads', () => {
    const rows = [
      occurrence('read', 'a', { unread: false }),
      occurrence('unsupported', 'a', { targetSupported: false }),
      occurrence('thread-b', 'a', { threadRootId: 'b' }),
      occurrence('thread-a', 'a', { threadRootId: 'a' })
    ];
    expect(
      nextUnreadSource([{ serverId: 'origin', roomIds: ['a'], occurrences: rows }])?.occurrence.id
    ).toBe('thread-a');
  });

  it('navigates from a draft, preserves it, and uses the remaining attention on the next press', async () => {
    const store = makeStore([occurrence('a'), occurrence('b')]);
    mocks.stores.set('origin', store);
    render(Harness);
    const draft = document.querySelector('textarea')!;
    draft.value = 'Keep my draft';
    const editorShortcut = vi.fn(() => {
      draft.value += ' duplicate';
    });
    draft.addEventListener('keydown', editorShortcut);
    draft.focus();
    expect(press().defaultPrevented).toBe(true);
    await expect
      .poll(() => ({ navigations: mocks.goto.mock.calls.length, errors: mocks.error.mock.calls }))
      .toEqual({ navigations: 1, errors: [] });
    expect(store.pendingHighlights.set).toHaveBeenCalledWith('a', null, 'event-a', 'a');
    expect(store.notifications.markOccurrenceRead).not.toHaveBeenCalled();
    store.attention.occurrences = [occurrence('b')];
    press();
    await expect.poll(() => mocks.goto.mock.calls.length).toBe(2);
    expect(store.pendingHighlights.set).toHaveBeenLastCalledWith('b', null, 'event-b', 'b');
    expect(draft.value).toBe('Keep my draft');
    expect(editorShortcut).not.toHaveBeenCalled();
    store.attention.occurrences = [];
    press();
    await expect.poll(() => mocks.info.mock.calls.length).toBe(1);
    expect(mocks.goto).toHaveBeenCalledTimes(2);
  });

  it('ignores repeat, composition, extra modifiers, and modal dialogs', async () => {
    const store = makeStore([occurrence('a')]);
    mocks.stores.set('origin', store);
    render(Harness);
    press({ repeat: true });
    press({ isComposing: true });
    press({ ctrlKey: true });
    const dialog = document.createElement('dialog');
    document.body.append(dialog);
    dialog.showModal();
    expect(press().defaultPrevented).toBe(false);
    dialog.remove();
    await Promise.resolve();
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(store.notifications.fetchAllPages).not.toHaveBeenCalled();
  });

  it('reports a failed page load without claiming the inbox is empty', async () => {
    const store = makeStore([]);
    store.notifications.fetchAllPages.mockRejectedValue(new Error('offline'));
    mocks.stores.set('origin', store);
    render(Harness);
    press();
    await expect.poll(() => mocks.error.mock.calls.length).toBe(1);
    expect(mocks.info).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it.each([false, true])('waits for older pages and fences privacy resets: %s', async (reset) => {
    const store = makeStore([]);
    let finish!: () => void;
    store.notifications.fetchAllPages.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        })
    );
    mocks.stores.set('origin', store);
    render(Harness);
    press();
    press();
    expect(store.notifications.fetchAllPages).toHaveBeenCalledTimes(1);
    expect(mocks.info).not.toHaveBeenCalled();
    if (reset) store.onReset.mock.calls[0]![0]({ privacy: true });
    store.attention.occurrences = [occurrence('older-page')];
    finish();
    await expect.poll(() => store.onReset.mock.results[0]!.value.mock.calls.length).toBe(1);
    expect(mocks.goto).toHaveBeenCalledTimes(reset ? 0 : 1);
    expect(mocks.info).not.toHaveBeenCalled();
  });

  it('offers the action in the quick switcher and closes the dialog before navigation', async () => {
    render(Harness);
    quickSwitcher.open();
    flushSync();
    const button = [...document.querySelectorAll('button')].find((node) =>
      node.textContent?.includes('Go to next unread source')
    )!;
    button.click();
    await expect.poll(() => mocks.info.mock.calls.length).toBe(1);
    expect(document.querySelector('dialog[open]')).toBeNull();
  });
});
