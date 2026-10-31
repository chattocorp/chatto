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
import ShortcutTextInput from '$lib/ui/form/ShortcutTextInput.svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import { cdp, page, userEvent } from 'vitest/browser';
import '../../app.css';

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
      occurrences: rows,
      hasLoaded: true,
      fetchPage: vi.fn(),
      fetchAllPages: vi.fn().mockResolvedValue(undefined),
      markOccurrenceRead: vi.fn().mockResolvedValue(undefined)
    },
    attention: { needsAttention: (row: NotificationOccurrenceItem) => row.unread },
    roomUnread: { roomIsUnread: vi.fn().mockReturnValue(false) },
    readViews: { covers: vi.fn().mockReturnValue(false) },
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
  ])('allows browser reload from a focused editor: %j', (shortcut) => {
    const { container } = render(Harness);
    container.querySelector('textarea')!.focus();
    expect(press({ altKey: false, shiftKey: false, ...shortcut }).defaultPrevented).toBe(false);
  });

  it('keeps ordinary typing available', () => {
    render(Harness);
    expect(press({ key: 'r', altKey: false, shiftKey: false }).defaultPrevented).toBe(false);
  });
});

describe('global keyboard shortcuts', () => {
  it('keeps the quick switcher open when an earlier native close event arrives', async () => {
    render(Harness);
    quickSwitcher.open();
    flushSync();
    const dialog = document.querySelector<HTMLDialogElement>('dialog.quick-switcher')!;
    expect(dialog.querySelector('[aria-label="Keyboard shortcuts"]')).toBeNull();
    quickSwitcher.close();
    flushSync();
    quickSwitcher.open();
    flushSync();
    dialog.dispatchEvent(new Event('close'));
    await expect.poll(() => quickSwitcher.visible).toBe(true);
    expect(dialog.open).toBe(true);
  });

  it.each([
    { help: false, touch: false },
    { help: false, touch: true },
    { help: true, touch: false },
    { help: true, touch: true }
  ])('keeps a full-screen mobile palette across resize: %j', async ({ help, touch }) => {
    await page.viewport(1280, 800);
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
    const viewport = Object.assign(new EventTarget(), { height: 800, offsetTop: 0 });
    vi.stubGlobal('visualViewport', viewport);
    const { container } = render(Harness);
    const opener = container.querySelector('textarea')!;
    opener.value = 'Keep the room draft';
    opener.focus();
    press({ key: 'k', altKey: false, ctrlKey: true, shiftKey: help });
    flushSync();
    const dialog = document.querySelector<HTMLDialogElement>('dialog[open]')!;
    const input = dialog.querySelector('input')!;
    await userEvent.fill(input, 'search');
    const back = dialog.querySelector<HTMLButtonElement>('button[aria-label="Back"]')!;
    try {
      for (const width of [390, 767, 768, 1280, 390]) {
        await page.viewport(width, 800);
        if (width < 768) {
          await expect.poll(() => dialog.matches(':modal')).toBe(false);
          await expect.poll(() => Math.round(dialog.getBoundingClientRect().width)).toBe(width);
          const headerHeight = touch ? 56 : 44;
          await expect
            .poll(() => Math.round(dialog.getBoundingClientRect().height))
            .toBe(800 - headerHeight);
          expect(dialog.getBoundingClientRect().top).toBeCloseTo(headerHeight, 0);
          await expect.element(back).toBeVisible();
          await expect.poll(() => back.getBoundingClientRect().height).toBe(touch ? 44 : 40);
          const frame = dialog.querySelector('.command-palette')!;
          expect(getComputedStyle(frame).fontSize).toBe('16px');
          expect(getComputedStyle(frame).gap).toBe('0px');
          for (const section of frame.querySelectorAll('.menu-section')) {
            expect(getComputedStyle(section).backgroundColor).toBe(
              getComputedStyle(frame).backgroundColor
            );
            expect(getComputedStyle(section).borderRadius).toBe('0px');
            expect(getComputedStyle(section).boxShadow).toBe('none');
          }
          expect(dialog.querySelector('.command-palette')!.scrollWidth).toBeLessThanOrEqual(width);
        } else {
          await expect.poll(() => dialog.matches(':modal')).toBe(true);
          await expect.element(back).not.toBeVisible();
          expect(dialog.getBoundingClientRect().width).toBeLessThan(width);
        }
        expect(dialog.querySelector('input')).toBe(input);
        expect(input.value).toBe('search');
        expect(document.activeElement).toBe(input);
      }
      viewport.height = 420;
      viewport.offsetTop = 12;
      viewport.dispatchEvent(new Event('resize'));
      const headerHeight = touch ? 56 : 44;
      await expect
        .poll(() => Math.round(dialog.getBoundingClientRect().height))
        .toBe(432 - headerHeight);
      expect(Math.round(dialog.getBoundingClientRect().top)).toBe(headerHeight);
      expect(back.getBoundingClientRect().top).toBeGreaterThanOrEqual(12);
      expect(input.getBoundingClientRect().bottom).toBeLessThan(432);
      back.click();
      await expect.poll(() => dialog.open).toBe(false);
      await expect.poll(() => document.activeElement).toBe(opener);
      expect(opener.value).toBe('Keep the room draft');
    } finally {
      await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
      vi.unstubAllGlobals();
      await page.viewport(1280, 800);
    }
  });

  it.each(['metaKey', 'ctrlKey'] as const)(
    'opens shortcut help with %s+Shift+K without changing a draft',
    async (modifier) => {
      render(Harness);
      const draft = document.querySelector('textarea')!;
      draft.value = 'Keep my draft';
      draft.focus();
      const event = press({ key: 'K', altKey: false, shiftKey: true, [modifier]: true });
      expect(event.defaultPrevented).toBe(true);
      flushSync();
      const dialog = document.querySelector('dialog[open]')!;
      expect(dialog.textContent).toContain('Keyboard shortcuts');
      expect(dialog.textContent).toContain('Open quick switcher');
      expect(dialog.textContent).toContain('Go to next unread source');
      expect(draft.value).toBe('Keep my draft');
      const close = [...dialog.querySelectorAll('button')].find(
        (button) => button.textContent?.trim() === 'Esc'
      )!;
      close.click();
      await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
      await expect.poll(() => dialog.querySelector('.command-palette')).toBeNull();
      expect(document.activeElement).toBe(draft);
    }
  );

  it('lists mounted filters and current editor preferences, and removes unmounted entries', async () => {
    const previousMode = userPreferences.composerSendMode;
    const previousEditor = userPreferences.composerEditor;
    userPreferences.composerSendMode = 'modifier-enter';
    userPreferences.composerEditor = 'visual';
    const view = render(Harness, { showFilter: true, showEditor: true });
    const help = async () => {
      document.querySelector('textarea')!.focus();
      press({ key: 'k', altKey: false, ctrlKey: true, shiftKey: true });
      await expect
        .poll(() => document.querySelector('dialog[open][aria-label="Keyboard shortcuts"]'))
        .not.toBeNull();
      return document.querySelector('dialog[open][aria-label="Keyboard shortcuts"]')!;
    };
    const close = async (dialog: Element) => {
      [...dialog.querySelectorAll('button')]
        .find((button) => button.textContent?.trim() === 'Esc')!
        .click();
      await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
      // Native close fires asynchronously after the open attribute is removed.
      await expect.poll(() => dialog.querySelector('.command-palette')).toBeNull();
    };
    try {
      let dialog = await help();
      expect(dialog.textContent).toContain('Filter permissions');
      expect(dialog.textContent).toContain('Bold');
      expect(dialog.textContent).toContain('Undo');
      const sendKeys = () =>
        [...dialog.querySelectorAll('dt')].find((row) => row.textContent === 'Send message')!
          .nextElementSibling!;
      expect(sendKeys().querySelectorAll('kbd')).toHaveLength(2);
      await close(dialog);
      userPreferences.composerSendMode = 'enter';
      userPreferences.composerEditor = 'markdown';
      dialog = await help();
      expect(sendKeys().querySelectorAll('kbd')).toHaveLength(1);
      expect(dialog.textContent).not.toContain('Heading');
      await close(dialog);
      await view.rerender({ showFilter: false, showEditor: false });
      dialog = await help();
      expect(dialog.textContent).not.toContain('Filter permissions');
      expect(dialog.textContent).not.toContain('Bold');
    } finally {
      userPreferences.composerSendMode = previousMode;
      userPreferences.composerEditor = previousEditor;
    }
  });

  it('leaves editor and navigation keys available to their local handlers', () => {
    render(Harness);
    const draft = document.querySelector('textarea')!;
    const localHandler = vi.fn();
    draft.addEventListener('keydown', localHandler);
    draft.focus();
    const keys: KeyboardEventInit[] = [
      { key: 'Escape' },
      { key: 'Tab' },
      { key: 'ArrowUp' },
      { key: 'ArrowDown' },
      { key: 'ArrowLeft', altKey: true },
      { key: 'Enter', shiftKey: true },
      { key: 'b', metaKey: true },
      { key: 'b', ctrlKey: true },
      { key: 'z', metaKey: true },
      { key: 'z', ctrlKey: true }
    ];
    for (const key of keys) {
      expect(press({ altKey: false, shiftKey: false, ...key }).defaultPrevented).toBe(false);
    }
    expect(localHandler).toHaveBeenCalledTimes(keys.length);
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(document.querySelector('dialog[open]')).toBeNull();
  });

  it.each(['metaKey', 'ctrlKey'] as const)(
    'preserves the filter input shortcut with %s',
    (modifier) => {
      render(Harness);
      render(ShortcutTextInput, {
        id: 'shortcut-filter',
        label: 'Filter',
        value: 'Find me',
        shortcutKey: '/'
      });
      document.querySelector('textarea')!.focus();
      press({ key: '/', altKey: false, shiftKey: false, [modifier]: true });
      const filter = document.getElementById('shortcut-filter') as HTMLInputElement;
      expect(document.activeElement).toBe(filter);
      expect(filter.selectionStart).toBe(0);
      expect(filter.selectionEnd).toBe(filter.value.length);
      flushSync();
      expect(document.querySelector('dialog[open]')).toBeNull();
    }
  );

  it.each(['metaKey', 'ctrlKey'] as const)(
    'opens the palette before editor bindings with %s',
    (modifier) => {
      render(Harness);
      const draft = document.querySelector('textarea')!;
      const editorShortcut = vi.fn();
      draft.addEventListener('keydown', editorShortcut);
      draft.focus();
      expect(
        press({ key: 'k', altKey: false, shiftKey: false, [modifier]: true }).defaultPrevented
      ).toBe(true);
      flushSync();
      expect(document.querySelector('dialog[open]')).not.toBeNull();
      expect(editorShortcut).not.toHaveBeenCalled();
    }
  );

  it.each(['metaKey', 'ctrlKey'] as const)(
    'submits forms only after local handlers decline %s+Enter',
    (modifier) => {
      render(Harness);
      const form = document.createElement('form');
      const input = document.createElement('input');
      form.append(input);
      document.body.append(form);
      const submit = vi.fn((event: Event) => event.preventDefault());
      form.addEventListener('submit', submit);
      input.focus();
      const shortcut = { key: 'Enter', altKey: false, shiftKey: false, [modifier]: true };
      try {
        press(shortcut);
        expect(submit).toHaveBeenCalledTimes(1);
        press({ ...shortcut, repeat: true });
        press({ ...shortcut, isComposing: true });
        press({ ...shortcut, shiftKey: true });
        expect(submit).toHaveBeenCalledTimes(1);
        input.addEventListener('keydown', (event) => event.preventDefault());
        press(shortcut);
        expect(submit).toHaveBeenCalledTimes(1);
      } finally {
        form.remove();
      }
    }
  );

  it('prioritises Important across servers, then room order and newest activity in a source', () => {
    const ambient = occurrence('ambient', 'a', {
      attentionLevel: NotificationAttentionLevel.AMBIENT
    });
    const old = occurrence('old', 'b', { createdAt: '2026-10-26T12:00:00Z' });
    const newest = occurrence('new', 'b');
    const servers = [
      { serverId: 'first', roomIds: ['a', 'b'], occurrences: [ambient], unreadRoomIds: ['a'] },
      {
        serverId: 'second',
        roomIds: ['b', 'a'],
        occurrences: [occurrence('a'), old, newest],
        unreadRoomIds: []
      }
    ];
    expect(nextUnreadSource(servers)?.occurrence?.id).toBe('new');
    servers[1]!.occurrences = [];
    expect(nextUnreadSource(servers)?.occurrence?.id).toBe('ambient');
    servers[0]!.occurrences = [];
    expect(nextUnreadSource(servers)).toEqual({
      serverId: 'first',
      roomId: 'a',
      threadRootId: null,
      occurrence: null
    });
  });

  it('excludes read and unsupported occurrences and keeps separate threads', () => {
    const rows = [
      occurrence('read', 'a', { unread: false }),
      occurrence('unsupported', 'a', { targetSupported: false }),
      occurrence('thread-b', 'a', { threadRootId: 'b' }),
      occurrence('thread-a', 'a', { threadRootId: 'a' })
    ];
    expect(
      nextUnreadSource([
        { serverId: 'origin', roomIds: ['a'], occurrences: rows, unreadRoomIds: [] }
      ])?.occurrence?.id
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
    store.notifications.occurrences = [occurrence('b')];
    press();
    await expect.poll(() => mocks.goto.mock.calls.length).toBe(2);
    expect(store.pendingHighlights.set).toHaveBeenLastCalledWith('b', null, 'event-b', 'b');
    expect(draft.value).toBe('Keep my draft');
    expect(editorShortcut).not.toHaveBeenCalled();
    store.notifications.occurrences = [];
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

  it('uses local notifications without loading pages, even before initial loading completes', async () => {
    const store = makeStore([occurrence('a')]);
    store.notifications.hasLoaded = false;
    mocks.stores.set('origin', store);
    render(Harness);
    press();
    await expect.poll(() => mocks.goto.mock.calls.length).toBe(1);
    expect(store.notifications.fetchPage).not.toHaveBeenCalled();
    expect(store.notifications.fetchAllPages).not.toHaveBeenCalled();
    expect(mocks.info).not.toHaveBeenCalled();
  });

  it('opens badge-only rooms in sidebar order and skips viewed rooms', async () => {
    const store = makeStore([]);
    store.roomUnread.roomIsUnread.mockReturnValue(true);
    store.readViews.covers.mockImplementation((roomId: string) => roomId === 'a');
    mocks.stores.set('origin', store);
    render(Harness);
    press();
    await expect.poll(() => mocks.goto.mock.calls.length).toBe(1);
    expect(mocks.goto.mock.calls[0]![0]).toMatch(/\/b$/);
    expect(store.notifications.fetchAllPages).not.toHaveBeenCalled();
    expect(store.notifications.markOccurrenceRead).not.toHaveBeenCalled();
    expect(store.pendingHighlights.set).not.toHaveBeenCalled();
    expect(mocks.info).not.toHaveBeenCalled();
  });

  it('continues through badge sources across servers as local read state changes', async () => {
    const first = makeStore([]);
    const second = makeStore([]);
    first.roomUnread.roomIsUnread.mockImplementation((roomId: string) => roomId === 'a');
    second.roomUnread.roomIsUnread.mockImplementation((roomId: string) => roomId === 'b');
    mocks.stores.set('first', first);
    mocks.stores.set('second', second);
    render(Harness);
    press();
    await expect.poll(() => mocks.goto.mock.calls.length).toBe(1);
    expect(mocks.goto.mock.calls[0]![0]).toBe('/chat/first.example.test/a');
    first.roomUnread.roomIsUnread.mockReturnValue(false);
    press();
    await expect.poll(() => mocks.goto.mock.calls.length).toBe(2);
    expect(mocks.goto.mock.calls[1]![0]).toBe('/chat/second.example.test/b');
    expect(first.notifications.fetchAllPages).not.toHaveBeenCalled();
    expect(second.notifications.fetchAllPages).not.toHaveBeenCalled();
  });

  it('selects Important notifications beyond the first 50 local rows before badge rooms', async () => {
    const store = makeStore([
      ...Array.from({ length: 50 }, (_, index) =>
        occurrence(`ambient-${index}`, 'a', {
          attentionLevel: NotificationAttentionLevel.AMBIENT
        })
      ),
      occurrence('important', 'b')
    ]);
    store.roomUnread.roomIsUnread.mockReturnValue(true);
    mocks.stores.set('origin', store);
    render(Harness);
    press();
    await expect.poll(() => mocks.goto.mock.calls.length).toBe(1);
    expect(store.pendingHighlights.set).toHaveBeenCalledWith(
      'b',
      null,
      'event-important',
      'important'
    );
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
