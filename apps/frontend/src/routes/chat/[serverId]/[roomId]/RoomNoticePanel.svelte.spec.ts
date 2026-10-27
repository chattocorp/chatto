import '../../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { signal } from '@chatto/client/reactivity';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { RoomNoticeStore } from '@chatto/client/room/notice';
import type { createRoomNoticesAPI } from '@chatto/client/api/roomNotices';
import type { ServerConnection } from '@chatto/client/server/serverConnection';
import { fakeServer, mockService, receivedRequest } from '@chatto/client/testing/fakeServer';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import RoomNoticePanel from './RoomNoticePanel.svelte';
import RoomNoticeBanner from './RoomNoticeBanner.svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';

const route = vi.hoisted(() => ({
  state: null as unknown as ReturnType<typeof signal<App.PageState>>
}));
vi.mock('$app/state', () => ({
  page: {
    get state() {
      return route.state.get();
    }
  }
}));
vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: (_url: string, state: App.PageState) => route.state.set(state)
}));

describe('RoomNoticePanel', () => {
  beforeEach(async () => {
    route.state = signal<App.PageState>({});
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it.each(['markdown', 'visual'] as const)(
    'completes mentions with the %s composer and renders their display names',
    async (editorKind) => {
      userPreferences.composerEditor = editorKind;
      const handlers = mockService(RoomService);
      handlers.getNotice.mockReturnValue({
        notice: { body: 'Ask @bob', version: 'v1', publicationId: 'p1' }
      });
      const config = fakeServer((router) => router.service(RoomService, handlers));
      const store = new RoomNoticeStore(
        {
          getAPI: (factory: typeof createRoomNoticesAPI) => factory(config)
        } as unknown as ServerConnection,
        'R1'
      );
      await store.refresh();
      const onMentionClick = vi.fn();
      const mentions = {
        members: [
          {
            id: 'bob-id',
            login: 'bob',
            displayName: 'Bob Example',
            avatarUrl: null,
            presenceStatus: 0
          }
        ],
        viewerLogin: 'bob',
        searchMembers: vi.fn(async () => [
          {
            id: 'alice-id',
            login: 'alice',
            displayName: 'Alice Remote',
            avatarUrl: null,
            presenceStatus: 0
          }
        ]),
        onMentionClick
      };
      const view = render(RoomNoticePanel, {
        store,
        serverId: 'test-server',
        canManage: true,
        mentions
      });
      await expect
        .element(page.getByText('@Bob Example', { exact: true }))
        .toHaveClass('mention-self');
      await page.getByText('@Bob Example', { exact: true }).click();
      expect(onMentionClick).toHaveBeenCalledWith('bob-id', expect.any(DOMRect));
      await page.getByRole('button', { name: 'Edit notice', exact: true }).click();
      const editor = page.getByRole('textbox', { name: 'Notice (Markdown)', exact: true });
      await editor.fill('@');
      await expect
        .element(page.getByTestId('mention-autocomplete'))
        .toHaveTextContent('Bob Example');
      await userEvent.keyboard('{Enter}');
      await expect.element(editor).toHaveTextContent('@bob');
      await expect.element(page.getByTestId('mention-autocomplete')).not.toBeInTheDocument();
      await editor.fill('@ali');
      await expect
        .element(page.getByTestId('mention-autocomplete'))
        .toHaveTextContent('Alice Remote');
      expect(mentions.searchMembers).toHaveBeenCalledWith('ali');
      await page.getByText('Alice Remote', { exact: true }).click();
      await expect.element(editor).toHaveTextContent('@alice');
      expect(handlers.publishNotice).not.toHaveBeenCalled();
      await view.unmount();
      const banner = render(RoomNoticeBanner, {
        store,
        serverId: 'test-server',
        viewerId: 'bob-id',
        mentions
      });
      await expect
        .element(page.getByText('@Bob Example', { exact: true }))
        .toHaveClass('mention-self');
      await banner.unmount();
      store.dispose();
    }
  );

  it('keeps a banner draft across reconnect and remote clear and counts trailing whitespace', async () => {
    userPreferences.composerEditor = 'markdown';
    const handlers = mockService(RoomService);
    handlers.getNotice.mockReturnValue({
      notice: { body: 'Active notice', version: 'v1', publicationId: 'p1' }
    });
    handlers.publishNotice.mockImplementation(() => {
      throw new ConnectError('Changed', Code.Aborted);
    });
    const config = fakeServer((router) => router.service(RoomService, handlers));
    const store = new RoomNoticeStore(
      {
        getAPI: (factory: typeof createRoomNoticesAPI) => factory(config)
      } as unknown as ServerConnection,
      'R1'
    );
    await store.refresh();
    render(RoomNoticeBanner, {
      store,
      serverId: 'test-server',
      viewerId: 'viewer',
      canManage: true
    });
    await page.getByRole('button', { name: 'Edit', exact: true }).click();
    const editor = page.getByRole('textbox', { name: 'Notice (Markdown)', exact: true });
    await editor.fill('x'.repeat(3999) + '\n\n');
    await expect.element(page.getByText('4001 / 4000', { exact: true })).toBeVisible();
    await expect.element(page.getByRole('alert')).toHaveTextContent('including whitespace');
    await expect
      .element(page.getByRole('button', { name: 'Announce', exact: true }))
      .toBeDisabled();
    await expect.element(page.getByRole('button', { name: 'Update', exact: true })).toBeDisabled();
    await editor.fill('Unsaved draft');
    const surface = document.querySelector('.origin-top');
    const modalState = route.state.get();
    store.reset();
    await expect.element(editor).toHaveTextContent('Unsaved draft');
    expect(document.querySelector('.origin-top')).toBe(surface);
    expect(route.state.get()).toBe(modalState);
    await store.refresh();
    await expect.element(editor).toHaveTextContent('Unsaved draft');
    expect(document.querySelector('.origin-top')).toBe(surface);
    handlers.getNotice.mockReturnValue({ notice: { body: '', version: 'v2' } });
    await store.refresh();
    await expect
      .element(page.getByRole('button', { name: 'Dismiss', exact: true }))
      .not.toBeInTheDocument();
    await expect.element(editor).toHaveTextContent('Unsaved draft');
    await page.getByRole('button', { name: 'Announce', exact: true }).click();
    await expect.element(page.getByRole('alert')).toHaveTextContent('Your draft is still here');
    expect(receivedRequest(handlers.publishNotice)).toMatchObject({
      expectedVersion: 'v1',
      body: 'Unsaved draft'
    });
    await expect.element(editor).toHaveTextContent('Unsaved draft');
    // Browser Back removes the still-owned editor route instead of a phantom step.
    route.state.set({});
    await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
    store.dispose();
  });

  it('confirms clearing the active notice and labels the empty editor Announce', async () => {
    userPreferences.composerEditor = 'markdown';
    const handlers = mockService(RoomService);
    handlers.getNotice.mockReturnValue({
      notice: { body: 'Active notice', version: 'v1', publicationId: 'p1' }
    });
    handlers.clearNotice.mockImplementation(() => {
      throw new ConnectError('Try again', Code.Unavailable);
    });
    const config = fakeServer((router) => router.service(RoomService, handlers));
    const store = new RoomNoticeStore(
      {
        getAPI: (factory: typeof createRoomNoticesAPI) => factory(config)
      } as unknown as ServerConnection,
      'R1'
    );
    await store.refresh();
    render(RoomNoticePanel, { store, serverId: 'test-server', canManage: true });
    const banner = render(RoomNoticeBanner, {
      store,
      serverId: 'test-server',
      viewerId: 'viewer',
      canManage: false
    });
    await expect
      .element(page.getByRole('button', { name: 'Edit', exact: true }))
      .not.toBeInTheDocument();
    await banner.rerender({ canManage: true });
    await page.getByRole('button', { name: 'Edit', exact: true }).click();
    await expect.poll(() => document.querySelectorAll('dialog[open]').length).toBe(1);
    await page.getByRole('button', { name: 'Clear', exact: true }).click();
    const confirmation = page.getByRole('dialog', { name: 'Clear room notice?', exact: true });
    await expect.element(confirmation).toHaveTextContent('immediately removes the active notice');
    expect(handlers.clearNotice).not.toHaveBeenCalled();
    await confirmation.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect.element(confirmation).not.toBeInTheDocument();
    expect(handlers.clearNotice).not.toHaveBeenCalled();
    await page.getByRole('button', { name: 'Clear', exact: true }).click();
    await confirmation.getByRole('button', { name: 'Clear', exact: true }).click();
    await vi.waitFor(() => expect(handlers.clearNotice).toHaveBeenCalledTimes(1));
    expect(receivedRequest(handlers.clearNotice)).toMatchObject({ expectedVersion: 'v1' });
    await expect.element(page.getByRole('alert')).toHaveTextContent('Network error');

    route.state.set({});
    await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
    handlers.getNotice.mockReturnValue({ notice: { body: '', version: 'v2' } });
    await store.refresh();
    await page.getByRole('button', { name: 'Edit notice', exact: true }).click();
    const announce = page.getByRole('button', { name: 'Announce', exact: true });
    await expect.element(announce).toBeDisabled();
    await page.getByRole('textbox', { name: 'Notice (Markdown)', exact: true }).fill('New notice');
    await expect.element(announce).toBeEnabled();
    store.dispose();
  });

  it.each(['markdown', 'visual'] as const)(
    'uses the %s composer and preserves conflicting drafts',
    async (editorKind) => {
      userPreferences.composerEditor = editorKind;
      const handlers = mockService(RoomService);
      handlers.getNotice.mockReturnValue({
        notice: { body: 'Original', version: 'v1', publicationId: 'p1' }
      });
      handlers.publishNotice.mockImplementation(() => {
        throw new ConnectError('Changed', Code.Aborted);
      });
      const config = fakeServer((router) => router.service(RoomService, handlers));
      const store = new RoomNoticeStore(
        {
          getAPI: (factory: typeof createRoomNoticesAPI) => factory(config)
        } as unknown as ServerConnection,
        'R1'
      );
      await store.refresh();
      render(RoomNoticePanel, { store, serverId: 'test-server', canManage: true });
      await page.getByRole('button', { name: 'Edit notice', exact: true }).click();
      await expect.element(page.getByRole('dialog')).toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Update', exact: true }))
        .toBeDisabled();
      await expect
        .element(page.getByRole('button', { name: 'Announce', exact: true }))
        .toBeDisabled();
      route.state.set({});
      await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
      await page.getByRole('button', { name: 'Edit notice', exact: true }).click();
      await page
        .getByRole('textbox', { name: 'Notice (Markdown)', exact: true })
        .fill('My correction');
      expect(
        document.querySelector(`dialog [data-composer-editor="${editorKind}"]`)
      ).not.toBeNull();
      const actions = Array.from(document.querySelectorAll('dialog button'))
        .map((button) => button.textContent?.trim())
        .filter((label) => ['Cancel', 'Clear', 'Update', 'Announce'].includes(label ?? ''));
      expect(actions).toEqual(['Cancel', 'Clear', 'Update', 'Announce']);
      await page.getByRole('button', { name: 'Update', exact: true }).click();
      await expect.element(page.getByRole('alert')).toHaveTextContent('Your draft is still here');
      await expect.element(page.getByRole('textbox')).toHaveTextContent('My correction');
      userPreferences.composerEditor = editorKind === 'markdown' ? 'visual' : 'markdown';
      await expect.element(page.getByRole('textbox')).toHaveTextContent('My correction');
      await vi.waitFor(() =>
        expect(
          document.querySelector(
            `dialog [data-composer-editor="${userPreferences.composerEditor}"]`
          )
        ).not.toBeNull()
      );
      expect(receivedRequest(handlers.publishNotice)).toMatchObject({
        expectedVersion: 'v1',
        republish: false,
        body: 'My correction'
      });
      expect(handlers.publishNotice).toHaveBeenCalledTimes(1);
      route.state.set({});
      await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
      await page.getByRole('button', { name: 'Edit notice', exact: true }).click();
      await page
        .getByRole('textbox', { name: 'Notice (Markdown)', exact: true })
        .fill('New announcement');
      await page.getByRole('button', { name: 'Announce', exact: true }).click();
      await vi.waitFor(() => expect(handlers.publishNotice).toHaveBeenCalledTimes(2));
      expect(receivedRequest(handlers.publishNotice, 1)).toMatchObject({
        expectedVersion: 'v1',
        republish: true,
        body: 'New announcement'
      });
      store.reset(true);
      await expect.element(page.getByRole('textbox')).not.toBeInTheDocument();
      store.dispose();
    }
  );
});
