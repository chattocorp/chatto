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
    await expect
      .element(page.getByText('Can read all messages — Rooms it has joined.'))
      .toBeVisible();
    await expect
      .element(page.getByText('Can post new messages — Direct messages it belongs to.'))
      .toBeVisible();
    await expect.element(page.getByRole('heading', { name: 'Inactive grants' })).toBeVisible();
    expect(mounted.container.textContent).toContain('owner’s current permissions');
    expect(mounted.container.textContent).toContain('edit and delete other users');
    expect(mounted.container.textContent).not.toContain('start DMs');
    expect(mocks.listPermissions).toHaveBeenCalledWith('bot', 0, expect.any(AbortSignal));
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
    await expect
      .element(page.getByText('Can read all messages — Rooms it has joined.'))
      .toBeVisible();
    mocks.listPermissions.mockRejectedValue(new Error('offline'));
    await queryClient.invalidateQueries({ queryKey: ['server', 'permission-test'] });
    await expect
      .element(page.getByRole('alert'))
      .toHaveTextContent('Could not load this bot’s permissions.');
    expect(mounted.container.textContent).not.toContain('Can read');
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
    await expect.element(page.getByRole('heading', { name: 'Inactive grants' })).toBeVisible();
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
    await expect
      .element(page.getByText('Can post new messages — Direct messages it belongs to.'))
      .toBeVisible();
    expect(mounted.container.querySelectorAll('li')).toHaveLength(2);
  });

  it('clears the previous bot while the next profile loads', async () => {
    mocks.listPermissions.mockResolvedValueOnce({
      permissions: [entry('message.read', 'server')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'first' });
    await expect
      .element(page.getByText('Can read all messages — Rooms it has joined.'))
      .toBeVisible();
    mocks.listPermissions.mockReturnValue(new Promise(() => {}));
    await mounted.rerender({ botId: 'second' });
    await expect.element(page.getByText('Loading...')).toBeVisible();
    expect(mounted.container.textContent).not.toContain('Can read');
    expect(mocks.listPermissions).toHaveBeenLastCalledWith('second', 0, expect.any(AbortSignal));
  });

  it('uses plain descriptions for echoes and calls', async () => {
    mocks.listPermissions.mockResolvedValue({
      permissions: [entry('message.echo', 'room'), entry('call.start', 'room')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect
      .element(page.getByText('Can show thread replies in the room timeline — #general.'))
      .toBeVisible();
    await expect.element(page.getByText('Can start calls — #general.')).toBeVisible();
  });

  it('renders German bot-specific descriptions', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    mocks.listPermissions.mockResolvedValue({
      permissions: [entry('message.read-interactions', 'room')],
      hasMore: false
    });
    mounted = render(BotPermissionSummary, { botId: 'bot' });
    await expect
      .element(page.getByRole('heading', { name: 'Was dieser Bot tun kann' }))
      .toBeVisible();
    await expect
      .element(
        page.getByText(
          'Darf: Threads lesen, die er erstellt hat oder in denen er direkt erwähnt wurde — #general.'
        )
      )
      .toBeVisible();
  });
});
