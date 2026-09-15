import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { queryClient } from '$lib/query/client';
import type { EffectivePermission } from '$lib/api-client/effectivePermissions';
import {
  compactEffectivePermissions,
  groupBotPermissions,
  inactiveBotGrants
} from './botPermissionText';

const mocks = vi.hoisted(() => ({
  listEffectivePermissions: vi.fn(),
  getUserPermissionMatrix: vi.fn(),
  serverId: 'effective-test'
}));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: mocks.serverId,
    connection: { queryScope: 'session', getAPI: () => mocks },
    store: {
      projection: {
        viewer: {
          user: { profile: { id: 'viewer' } },
          viewerPermissions: { permissions: [] }
        }
      }
    }
  })
}));
import BotPermissionSummary from './BotPermissionSummary.svelte';

function grant(
  permission: string,
  scope: EffectivePermission['scope'] = 'server',
  overrides: Partial<EffectivePermission> = {}
): EffectivePermission {
  return {
    permission,
    scope,
    scopeId: '',
    scopeName: '',
    parentGroupId: '',
    coversDescendants: true,
    ...overrides
  };
}
let view: ReturnType<typeof render> | undefined;
beforeEach(() => {
  vi.resetAllMocks();
  mocks.serverId = `effective-test-${crypto.randomUUID()}`;
  queryClient.clear();
  localStorage.clear();
  queryClient.setQueryDefaults(['server', mocks.serverId], { retry: false });
});
afterEach(() => {
  queryClient.clear();
});

it('groups effective grants without issuing an admin read for ordinary members', async () => {
  mocks.listEffectivePermissions.mockResolvedValue([
    grant('message.read'),
    grant('message.read-interactions'),
    grant('message.read', 'dm'),
    grant('room.list'),
    grant('room.join')
  ]);
  view = render(BotPermissionSummary, { botId: 'bot', botOwnerId: 'owner' });
  await expect.element(page.getByText('Browse and join rooms')).toBeVisible();
  await expect.element(page.getByText('Rooms it has joined', { exact: true })).toBeVisible();
  await expect.element(page.getByText('Its direct messages', { exact: true })).toBeVisible();
  expect(mocks.getUserPermissionMatrix).not.toHaveBeenCalled();
  expect(view.container.textContent).not.toContain('Inactive grants');
});

it('keeps the disclosure collapsed across profile mounts', async () => {
  mocks.listEffectivePermissions.mockResolvedValue([grant('message.read')]);
  view = render(BotPermissionSummary, { botId: 'bot' });
  await expect.element(page.getByText('Read all messages')).toBeVisible();
  await page.getByRole('button', { name: 'What it can do' }).click();
  await expect.element(page.getByText('Read all messages')).not.toBeInTheDocument();
  await view.unmount();
  view = render(BotPermissionSummary, { botId: 'bot' });
  await expect
    .element(page.getByRole('button', { name: 'What it can do' }))
    .toHaveAttribute('aria-expanded', 'false');
});

it('hides stale effective grants after a read error and permits retry', async () => {
  mocks.listEffectivePermissions.mockResolvedValue([grant('message.read')]);
  view = render(BotPermissionSummary, { botId: 'bot' });
  await expect.element(page.getByText('Read all messages')).toBeVisible();
  mocks.listEffectivePermissions.mockRejectedValue(new Error('offline'));
  await queryClient.invalidateQueries({ queryKey: ['server', mocks.serverId] });
  await expect.element(page.getByRole('alert')).toBeVisible();
  expect(view.container.textContent).not.toContain('Read all messages');
  mocks.listEffectivePermissions.mockResolvedValue([]);
  await page.getByRole('button', { name: 'Try Again' }).click();
  await expect.element(page.getByText('No active permissions are visible to you.')).toBeVisible();
});

it('shows the complete result from one request without a load-more control', async () => {
  mocks.listEffectivePermissions.mockResolvedValue([grant('message.read'), grant('message.post')]);
  view = render(BotPermissionSummary, { botId: 'bot' });
  await expect.element(page.getByText('Read all messages')).toBeVisible();
  await expect.element(page.getByText('Post new messages')).toBeVisible();
  expect(mocks.listEffectivePermissions).toHaveBeenCalledExactlyOnceWith(
    'bot',
    expect.any(AbortSignal)
  );
  await expect
    .element(page.getByRole('button', { name: 'Show more permissions' }))
    .not.toBeInTheDocument();
});

it('reads unavailable grants separately for the owner and hides them on ownership loss', async () => {
  mocks.listEffectivePermissions.mockResolvedValue([]);
  mocks.getUserPermissionMatrix.mockResolvedValue({
    applicablePermissions: ['message.manage'],
    scopes: [{ id: 'dm', kind: 'DM', label: 'Direct messages', parentGroupId: '' }],
    cells: [{ permission: 'message.manage', scopeId: 'dm', override: 'ALLOW', effective: 'NONE' }],
    page: { hasMore: false }
  });
  view = render(BotPermissionSummary, { botId: 'bot', botOwnerId: 'viewer' });
  await expect.element(page.getByText('Inactive grants')).toBeVisible();
  await page.getByText('Inactive grants').click();
  await expect.element(page.getByText('Manage messages')).toBeVisible();
  await view.rerender({ botId: 'bot', botOwnerId: 'someone-else' });
  await expect.element(page.getByText('Inactive grants')).not.toBeInTheDocument();
});

it('does not claim global access when a hidden descendant restricts it', () => {
  const result = compactEffectivePermissions([
    grant('message.read', 'server', { coversDescendants: false }),
    grant('message.read', 'room', { scopeId: 'visible', scopeName: 'general' }),
    grant('message.read', 'dm')
  ]);
  expect(result.map((x) => x.scope)).toEqual(['room', 'dm']);
});

it('combines inherited scopes and included permissions only under proven coverage', () => {
  const result = compactEffectivePermissions([
    grant('message.read'),
    grant('message.read', 'room', { scopeId: 'room' }),
    grant('message.read-interactions', 'room', { scopeId: 'room' }),
    grant('message.read', 'dm')
  ]);
  expect(result.map((x) => [x.permission, x.scope])).toEqual([
    ['message.read', 'server'],
    ['message.read', 'dm']
  ]);
  expect(groupBotPermissions(result).map((x) => x.icon)).toEqual([
    'icon-[uil--users-alt]',
    'icon-[uil--comments]'
  ]);
});

it('finds inherited inactive grants from the admin matrix without treating unconfigured cells as grants', () => {
  const result = inactiveBotGrants({
    applicablePermissions: ['message.read', 'message.post'],
    scopes: [
      { id: 'server', kind: 'SERVER', label: 'Server', parentGroupId: '' },
      { id: 'room:r', kind: 'ROOM', label: 'general', parentGroupId: 'g' }
    ],
    cells: [
      { permission: 'message.read', scopeId: 'server', override: 'ALLOW', effective: 'ALLOW' },
      { permission: 'message.read', scopeId: 'room:r', override: 'NONE', effective: 'NONE' },
      { permission: 'message.post', scopeId: 'room:r', override: 'NONE', effective: 'NONE' }
    ]
  });
  expect(result).toEqual([
    { permission: 'message.read', scope: 'room', scopeId: 'r', scopeName: 'general', active: false }
  ]);
});
