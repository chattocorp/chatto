import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { queryClient } from '$lib/query/client';
import { getUserStore, resetUserStoresForTests } from '$lib/state/server/users.svelte';
import { userProfileFixture } from '$lib/test-utils/userProfile';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
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
const mocks = { batchGetUsers: vi.fn() };
let server: TestServerScope;
const users = () => getUserStore(server.serverId, server.scope.connection.queryScope);
let view: ReturnType<typeof render> | undefined;
beforeEach(() => {
  vi.resetAllMocks();
  server = createTestServerScope({
    serverId: 'owner-test',
    permissions: { loaded: false },
    api: {
      batchGetUsers: async (ids: string[]) => {
        const result = await mocks.batchGetUsers(ids);
        for (const user of result) users().set(user.id, userProfileFixture(user));
        return result;
      }
    },
    store: {
      get projection() {
        return { users: users() };
      }
    }
  });
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
  await expect.element(page.getByRole('status', { name: 'Loading...' })).toBeInTheDocument();
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
