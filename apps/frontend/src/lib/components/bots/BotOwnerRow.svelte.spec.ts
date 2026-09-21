import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { queryClient } from '$lib/query/client';
import { getUserStore, resetUserStoresForTests } from '$lib/state/server/users.svelte';
import { userProfileFixture } from '$lib/test-utils/userProfile';

const mocks = vi.hoisted(() => ({ batchGetUsers: vi.fn() }));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'owner-test',
    connection: { queryScope: 'session', getAPI: () => ({
      batchGetUsers: async (ids: string[]) => {
        const users = await mocks.batchGetUsers(ids);
        const store = getUserStore('owner-test', 'session');
        for (const user of users) store.set(user.id, userProfileFixture(user));
        return users;
      }
    }) },
    isCurrent: () => true,
    store: {
      permissions: { loaded: false },
      get projection() { return { users: getUserStore('owner-test', 'session') }; }
    }
  })
}));
vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_id: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_id: string, fallback: string | null) => fallback,
  getLiveBio: (_id: string, fallback: string | null) => fallback,
  getLiveTimezone: (_id: string, fallback: string | null) => fallback,
  getLiveCustomStatus: () => null
}));
import BotOwnerRow from './BotOwnerRow.svelte';

const owner = {
  id: 'owner',
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
  deleted: false
};
let view: ReturnType<typeof render> | undefined;
beforeEach(() => {
  vi.resetAllMocks();
  queryClient.clear();
  resetUserStoresForTests();
  queryClient.setQueryDefaults(['server', 'owner-test'], { retry: false });
});
afterEach(() => {
  view?.unmount();
  queryClient.clear();
});

it('shows the human owner as a compact profile button', async () => {
  mocks.batchGetUsers.mockResolvedValue([owner]);
  view = render(BotOwnerRow, { ownerId: 'owner' });
  await expect.element(page.getByText('Owned by')).toBeVisible();
  await expect.element(page.getByRole('button', { name: 'View profile of Alice' })).toBeVisible();
  expect(mocks.batchGetUsers).toHaveBeenCalledWith(['owner']);
});

it('clears the previous owner while another owner loads', async () => {
  mocks.batchGetUsers.mockResolvedValueOnce([owner]).mockReturnValue(new Promise(() => {}));
  view = render(BotOwnerRow, { ownerId: 'owner' });
  await expect.element(page.getByText('Alice', { exact: true })).toBeVisible();
  await view.rerender({ ownerId: 'next-owner' });
  await expect.element(page.getByText('Loading...')).toBeVisible();
  await expect.element(page.getByText('Alice', { exact: true })).not.toBeInTheDocument();
});

it('does not offer a profile action for a deleted owner', async () => {
  mocks.batchGetUsers.mockResolvedValue([{ ...owner, deleted: true }]);
  view = render(BotOwnerRow, { ownerId: 'owner' });
  await expect.element(page.getByText('[deleted user]')).toBeVisible();
  expect(view.container.querySelector('button')).toBeNull();
});

it('keeps a failed owner lookup from hiding the rest of the profile', async () => {
  mocks.batchGetUsers.mockRejectedValue(new Error('offline'));
  view = render(BotOwnerRow, { ownerId: 'owner' });
  await expect.element(page.getByText('Unknown user')).toBeVisible();
  await expect.element(page.getByText('Owned by')).toBeVisible();
  expect(view.container.querySelector('button')).toBeNull();
});
