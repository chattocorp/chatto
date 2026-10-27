import '../../../../app.css';
import { beforeEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { resetRoomGroupCollapseForTests } from '$lib/components/chat/roomGroupCollapse';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import RoomInformationPanel from './RoomInformationPanel.svelte';
import { RoomNotice } from '@chatto/api-types/api/v1/rooms_pb';
import { RoomNoticeStore } from '@chatto/client/room/notice';
import type { ServerConnection } from '@chatto/client/server/serverConnection';

describe('RoomInformationPanel', () => {
  beforeEach(async () => {
    localStorage.clear();
    resetRoomGroupCollapseForTests();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('hides an empty notice section from readers but keeps it available to editors', async () => {
    let notice = new RoomNotice();
    const store = new RoomNoticeStore(
      { getAPI: () => ({ get: async () => notice }) } as unknown as ServerConnection,
      'R1'
    );
    await store.refresh();
    const view = render(RoomInformationPanel, {
      serverId: 'test-server',
      roomName: 'general',
      roomDescription: '',
      noticeStore: store
    });
    const section = page.getByRole('button', { name: 'Room notice', exact: true });
    await expect.element(section).not.toBeInTheDocument();
    await view.rerender({ canManageNotice: true });
    await expect.element(section).toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Edit notice', exact: true }))
      .toBeVisible();
    await view.rerender({ canManageNotice: false });
    notice = new RoomNotice({ body: 'Published notice', version: 'v1', publicationId: 'p1' });
    await store.refresh();
    await expect.element(section).toBeVisible();
    await expect.element(page.getByText('Published notice', { exact: true })).toBeVisible();
    notice = new RoomNotice();
    await store.refresh();
    await expect.element(section).not.toBeInTheDocument();
    store.dispose();
  });

  it('renders description Markdown through the message renderer', async () => {
    const { container } = render(RoomInformationPanel, {
      serverId: 'test-server',
      roomName: 'general',
      roomDescription:
        'Welcome to **general**.\n\n> Ask for help here.\n\n- Share ideas\n- Read `guidelines`'
    });
    await expect
      .poll(() => container.querySelector('blockquote')?.textContent)
      .toContain('Ask for help here.');
    expect(container.querySelector('strong')?.textContent).toBe('general');
    expect(container.querySelectorAll('li')).toHaveLength(2);
    expect(container.querySelector('code')?.textContent).toBe('guidelines');
  });

  it('shows creation time and updates the member count without treating missing data as zero', async () => {
    const view = render(RoomInformationPanel, {
      serverId: 'test-server',
      roomName: 'general',
      roomDescription: '',
      roomCreatedAt: '2020-01-02T03:04:05.000Z',
      memberCount: 42
    });
    await expect.element(page.getByText('42', { exact: true })).toBeVisible();
    expect(view.container.querySelector('time')?.getAttribute('datetime')).toBe(
      '2020-01-02T03:04:05.000Z'
    );
    expect(view.container.querySelector('time')?.textContent).toContain('ago');
    await view.rerender({ memberCount: 0 });
    await expect.element(page.getByText('0', { exact: true })).toBeVisible();
    await view.rerender({ memberCount: null, roomCreatedAt: null });
    await expect.element(page.getByText('0', { exact: true })).not.toBeInTheDocument();
    await expect.poll(() => view.container.querySelector('time')).toBeNull();
  });

  it('shows current room metadata and collapses each section independently', async () => {
    const view = render(RoomInformationPanel, {
      serverId: 'test-server',
      roomName: 'General',
      roomDescription: 'A place for everyone.'
    });
    const basic = page.getByRole('button', { name: 'General', exact: true });
    const pins = page.getByRole('button', { name: 'Pinned messages', exact: true });
    const description = page.getByRole('button', { name: 'Description', exact: true });
    await expect.element(description).not.toBeInTheDocument();
    await expect.element(page.getByText('A place for everyone.', { exact: true })).toBeVisible();
    await expect.element(basic).toHaveAttribute('aria-expanded', 'true');
    await expect.element(pins).toHaveAttribute('aria-expanded', 'true');
    await expect.element(page.getByText('#', { exact: true })).toBeVisible();

    await basic.click();
    await expect.element(basic).toHaveAttribute('aria-expanded', 'false');
    await expect.element(pins).toHaveAttribute('aria-expanded', 'true');
    expect(view.container.querySelector('.prose')?.closest('[inert]')).not.toBeNull();
    await pins.click();
    await basic.click();
    await expect.element(basic).toHaveAttribute('aria-expanded', 'true');
    await expect.element(pins).toHaveAttribute('aria-expanded', 'false');

    await view.rerender({
      roomName: 'Support',
      roomDescription: 'Ask for help here.',
      isUniversal: true
    });
    await expect.element(page.getByRole('img', { name: 'Universal room' })).toBeVisible();
    await expect.element(page.getByText('Support', { exact: true })).toBeVisible();
    await expect.element(page.getByText('Ask for help here.', { exact: true })).toBeVisible();
    await expect
      .element(page.getByText('A place for everyone.', { exact: true }))
      .not.toBeInTheDocument();
  });
});
