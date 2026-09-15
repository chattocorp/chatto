import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { queryClient } from '$lib/query/client';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import type { BotPermission } from '$lib/api-client/bots';

const mocks = vi.hoisted(() => ({ listPermissions: vi.fn() }));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'permission-test',
    connection: { queryScope: 'session', getAPI: () => mocks }
  })
}));
import BotPermissionSummary from './BotPermissionSummary.svelte';

function entry(permission: string, scope: BotPermission['scope'], active = true): BotPermission {
  return {
    permission,
    scope,
    scopeId: scope === 'room' ? 'general' : '',
    scopeName: scope === 'room' ? 'general' : '',
    active
  };
}

let mounted: ReturnType<typeof render> | undefined;
describe('bot permission summary', () => {
  beforeEach(async () => {
    vi.resetAllMocks();
    queryClient.clear();
    queryClient.setQueryDefaults(['server', 'permission-test'], { retry: false });
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    queryClient.clear();
  });

  it('renders readable active and inactive permissions with distinct channel and DM scopes', async () => {
    mocks.listPermissions.mockResolvedValue({
      permissions: [
        entry('message.read', 'server'),
        entry('message.post', 'dm'),
        entry('message.manage', 'room', false)
      ],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect.element(page.getByText('Read all messages')).toBeVisible();
    await expect.element(page.getByText('Post new messages')).toBeVisible();
    await expect.element(page.getByText('Inactive grants', { exact: true })).toBeVisible();
    await expect.element(page.getByRole('heading', { name: 'Rooms it has joined' })).toBeVisible();
    await expect.element(page.getByRole('heading', { name: 'Its direct messages' })).toBeVisible();
    await expect.element(page.getByText('Manage messages', { exact: true })).not.toBeVisible();
    await page.getByText('Inactive grants', { exact: true }).click();
    await expect.element(page.getByText('Manage messages', { exact: true })).toBeVisible();
    expect(mounted.container.textContent).toContain('owner’s current permissions');
    expect(mounted.container.textContent).toContain('Manage messages');
    expect(mounted.container.textContent).not.toContain('start DMs');
    expect(mocks.listPermissions).toHaveBeenCalledWith('bot', 0, expect.any(AbortSignal));
  });

  it('groups actions by scope and combines browsing and joining only within that scope', async () => {
    mocks.listPermissions.mockResolvedValue({
      permissions: [
        entry('room.join', 'server'),
        entry('room.list', 'server'),
        entry('message.read', 'server'),
        entry('message.post-in-thread', 'server'),
        entry('message.read', 'dm'),
        entry('message.post-in-thread', 'dm'),
        entry('room.list', 'room'),
        entry('room.join', 'room', false)
      ],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect.element(page.getByText('Browse and join rooms', { exact: true })).toBeVisible();
    await expect.element(page.getByText('Browse rooms', { exact: true })).toBeVisible();
    const headings = [...mounted.container.querySelectorAll('h4')].map((el) =>
      el.textContent?.trim()
    );
    expect(headings).toEqual([
      'Rooms',
      'Rooms it has joined',
      '#general',
      'Its direct messages',
      '#general'
    ]);
    expect(mounted.container.querySelectorAll('li')).toHaveLength(7);
    expect(mounted.container.querySelector('details')?.open).toBe(false);
    await expect
      .element(page.getByRole('button', { name: 'More information' }))
      .not.toBeInTheDocument();
  });

  it('keeps rooms with equal names in separate groups', async () => {
    mocks.listPermissions.mockResolvedValue({
      permissions: [
        entry('room.list', 'room'),
        { ...entry('room.join', 'room'), scopeId: 'another' }
      ],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect.element(page.getByText('Browse rooms', { exact: true })).toBeVisible();
    expect(mounted.container.querySelectorAll('h4')).toHaveLength(2);
    expect(mounted.container.textContent).not.toContain('Browse and join');
  });

  it('shows loading, then an empty result without an inactive section', async () => {
    let resolve!: (value: unknown) => void;
    mocks.listPermissions.mockReturnValue(
      new Promise((done) => {
        resolve = done;
      })
    );
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect.element(page.getByText('Loading...')).toBeVisible();
    resolve({ permissions: [], hasMore: false });
    await expect.element(page.getByText('No active permissions are visible to you.')).toBeVisible();
    expect(mounted.container.textContent).not.toContain('Inactive grants');
  });

  it('hides stale grants after a failed refresh and supports retry', async () => {
    mocks.listPermissions.mockResolvedValue({
      permissions: [entry('message.read', 'server')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect.element(page.getByText('Read all messages')).toBeVisible();
    mocks.listPermissions.mockRejectedValue(new Error('offline'));
    await queryClient.invalidateQueries({ queryKey: ['server', 'permission-test'] });
    await expect
      .element(page.getByRole('alert'))
      .toHaveTextContent('Could not load this bot’s permissions.');
    expect(mounted.container.textContent).not.toContain('Read all messages');
    mocks.listPermissions.mockResolvedValue({ permissions: [], hasMore: false });
    await page.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(page.getByText('No active permissions are visible to you.')).toBeVisible();
  });

  it('loads another page and keeps inactive grants separate', async () => {
    mocks.listPermissions
      .mockResolvedValueOnce({ permissions: [entry('message.read', 'room')], hasMore: true })
      .mockResolvedValueOnce({
        permissions: [entry('message.post', 'room', false)],
        hasMore: false
      });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await page.getByRole('button', { name: 'Show more permissions' }).click();
    await expect.element(page.getByText('Inactive grants', { exact: true })).toBeVisible();
    expect(mocks.listPermissions).toHaveBeenLastCalledWith('bot', 1, expect.any(AbortSignal));
  });

  it('combines overlapping pages without duplicate bullets', async () => {
    mocks.listPermissions
      .mockResolvedValueOnce({ permissions: [entry('message.read', 'room')], hasMore: true })
      .mockResolvedValueOnce({
        permissions: [entry('message.read', 'room'), entry('message.post', 'dm')],
        hasMore: false
      });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await page.getByRole('button', { name: 'Show more permissions' }).click();
    await expect.element(page.getByText('Post new messages')).toBeVisible();
    expect(mounted.container.querySelectorAll('li')).toHaveLength(2);
  });

  it('clears the previous bot while the next profile loads', async () => {
    mocks.listPermissions.mockResolvedValueOnce({
      permissions: [entry('message.read', 'server')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'first' });
    await expect.element(page.getByText('Read all messages')).toBeVisible();
    mocks.listPermissions.mockReturnValue(new Promise(() => {}));
    await mounted.rerender({ botId: 'second' });
    await expect.element(page.getByText('Loading...')).toBeVisible();
    expect(mounted.container.textContent).not.toContain('Read all messages');
    expect(mocks.listPermissions).toHaveBeenLastCalledWith('second', 0, expect.any(AbortSignal));
  });

  it('uses plain descriptions for echoes and calls', async () => {
    mocks.listPermissions.mockResolvedValue({
      permissions: [entry('message.echo', 'room'), entry('call.start', 'room')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect.element(page.getByText('Show thread replies in the room timeline')).toBeVisible();
    await expect.element(page.getByText('Start calls')).toBeVisible();
  });

  it('renders German bot-specific descriptions', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    mocks.listPermissions.mockResolvedValue({
      permissions: [entry('message.read-interactions', 'room')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect.element(page.getByRole('button', { name: 'Was er tun kann' })).toBeVisible();
    await expect
      .element(
        page.getByText('Threads lesen, die er erstellt hat oder in denen er direkt erwähnt wurde')
      )
      .toBeVisible();
  });

  it('collapses the complete permission section and remembers the choice', async () => {
    mocks.listPermissions.mockResolvedValue({
      permissions: [entry('message.read', 'server')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'collapse-test' });
    const toggle = page.getByRole('button', { name: 'What it can do' });
    await expect.element(page.getByText('Read all messages', { exact: true })).toBeVisible();
    await toggle.click();
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect
      .element(page.getByText('Read all messages', { exact: true }))
      .not.toBeInTheDocument();
    mounted.unmount();
    mounted = render(BotPermissionSummary, { botId: 'collapse-test' });
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
    await toggle.click();
    await expect.element(page.getByText('Read all messages', { exact: true })).toBeVisible();
  });
});
