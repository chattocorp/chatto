import { beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { testSnippet } from '$lib/test-utils';
import { CurrentUserState, type CurrentUser } from '$lib/auth/currentUser.svelte';

const mocks = vi.hoisted(() => ({ currentUser: null as unknown as CurrentUserState }));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({ store: { currentUser: mocks.currentUser } })
}));
import Layout from './+layout.svelte';

beforeEach(() => {
  mocks.currentUser = new CurrentUserState();
});

it('waits for account data and preserves form edits during refresh', async () => {
  const view = render(Layout, {
    props: { children: testSnippet('<input data-testid="draft" />') }
  });
  expect(view.container.querySelector('input')).toBeNull();
  mocks.currentUser.accept({ id: 'U1', login: 'alice' } as CurrentUser);
  await tick();
  const input = view.container.querySelector('input')!;
  input.value = 'Unsent change';
  mocks.currentUser.loading = true;
  await tick();
  expect(view.container.querySelector('input')).toBe(input);
  mocks.currentUser.accept({ id: 'U1', login: 'alice-new' } as CurrentUser);
  await tick();
  expect(input.value).toBe('Unsent change');
  mocks.currentUser.reset();
  await tick();
  expect(view.container.querySelector('input')).toBeNull();
});
