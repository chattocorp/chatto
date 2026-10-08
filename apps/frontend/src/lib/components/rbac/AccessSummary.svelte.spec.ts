import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import type { AccessSummary as Summary } from '@chatto/client/api/permissions';

const mocks = { getAccessSummary: vi.fn() };
let server: TestServerScope;
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
import AccessSummary from './AccessSummary.svelte';

function summary(overrides: Partial<Summary> = {}): Summary {
  return {
    everyoneCanList: false,
    everyoneCanJoin: false,
    everyoneCanRead: false,
    rolesCanList: [],
    rolesCanJoin: [],
    ...overrides
  };
}

beforeEach(async () => {
  vi.resetAllMocks();
  await loadLocaleMessages('en-GB');
  setReactiveLocale('en-GB');
  server = createTestServerScope({
    serverId: `access-summary-${crypto.randomUUID()}`,
    viewer: { id: 'viewer' },
    api: mocks
  });
  queryClient.clear();
  queryClient.setQueryDefaults(['server', server.serverId], { retry: false });
});

afterEach(() => {
  queryClient.clear();
});

it('warns when nobody else can join a new room', async () => {
  mocks.getAccessSummary.mockResolvedValue(summary());
  render(AccessSummary, { roomId: 'room-1' });

  await expect.element(page.getByText(/Nobody can find, join, or read this room yet/)).toBeVisible();
  expect(mocks.getAccessSummary).toHaveBeenCalledWith(
    { roomId: 'room-1', groupId: null },
    expect.anything()
  );
});

it('names the roles that can join', async () => {
  mocks.getAccessSummary.mockResolvedValue(summary({ rolesCanJoin: ['engineering', 'staff'] }));
  render(AccessSummary, { roomId: 'room-1' });

  await expect
    .element(page.getByText('Only members of these roles can join this room: engineering, staff.'))
    .toBeVisible();
});

it('tells when everyone can find and join the rooms of a room group', async () => {
  mocks.getAccessSummary.mockResolvedValue(
    summary({ everyoneCanList: true, everyoneCanJoin: true, everyoneCanRead: true })
  );
  render(AccessSummary, { groupId: 'group-1' });

  await expect
    .element(page.getByText('Everyone can find and join the rooms in this room group.'))
    .toBeVisible();
});

it('tells when everyone can join a room that is not listed for everyone', async () => {
  mocks.getAccessSummary.mockResolvedValue(
    summary({ everyoneCanJoin: true, everyoneCanRead: true })
  );
  render(AccessSummary, { roomId: 'room-1' });

  await expect.element(page.getByText(/does not show in the room list/)).toBeVisible();
});

it('warns when everyone can join a room but cannot read it', async () => {
  mocks.getAccessSummary.mockResolvedValue(
    summary({ everyoneCanList: true, everyoneCanJoin: true })
  );
  render(AccessSummary, { roomId: 'room-1' });

  await expect
    .element(page.getByText(/Everyone can join this room, but cannot read it/))
    .toBeVisible();
});
