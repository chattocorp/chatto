import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { CurrentUserState, type CurrentUser } from '$lib/auth/currentUser.svelte';
import { q } from '$lib/test-utils';

const mocks = vi.hoisted(() => ({
  currentUser: null as unknown as CurrentUserState,
  updateSettings: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'remote',
    store: { currentUser: mocks.currentUser },
    connection: {
      getAPI: () => ({ updateSettings: mocks.updateSettings })
    },
    isCurrent: () => true
  })
}));

import PreferencesPage from './+page.svelte';

function currentUser(settings: NonNullable<CurrentUser['settings']>): CurrentUser {
  return {
    id: 'user-1',
    login: 'alice',
    displayName: 'Alice',
    avatarUrl: null,
    customStatus: null,
    presenceStatus: PresenceStatus.ONLINE,
    hasVerifiedEmail: true,
    hasPassword: true,
    viewerCanDeleteAccount: true,
    lastLoginChange: null,
    settings
  };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function buttonWithText(container: Element, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
    (candidate) => candidate.textContent?.includes(text)
  );
  if (!button) throw new Error(`Button with text "${text}" not found`);
  return button;
}

describe('Time and region settings page', () => {
  beforeEach(() => {
    mocks.currentUser = new CurrentUserState();
    mocks.updateSettings.mockReset();
  });

  it('hydrates untouched edit buffers when remote viewer settings finish loading', async () => {
    const { container } = render(PreferencesPage);
    await settle();

    const timezoneInput = q(container, '[data-testid="timezone-input"]') as HTMLInputElement;
    const saveButton = buttonWithText(container, 'Save');
    expect(container.querySelectorAll('.panel-shell')).toHaveLength(1);
    await expect.element(timezoneInput).toHaveValue('');
    await expect.element(timezoneInput).toBeDisabled();
    await expect.element(buttonWithText(container, '24-hour')).toBeDisabled();
    await expect.element(saveButton).toBeDisabled();

    mocks.currentUser.user = currentUser({
      timezone: 'Pacific/Honolulu',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
      shareTimezone: false
    });
    mocks.currentUser.loading = false;
    await settle();

    await expect.element(timezoneInput).toHaveValue('Pacific/Honolulu');
    await expect.element(timezoneInput).toBeEnabled();
    await expect
      .element(buttonWithText(container, '24-hour'))
      .toHaveAttribute('aria-checked', 'true');
    await expect.element(saveButton).toBeDisabled();
  });

  it('saves a timezone-sharing change as a sparse patch', async () => {
    mocks.currentUser.user = currentUser({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
      shareTimezone: false
    });
    mocks.currentUser.loading = false;
    mocks.updateSettings.mockResolvedValue({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
      shareTimezone: true
    });

    const { container } = render(PreferencesPage);
    await settle();
    const sharing = q(container, '#share-timezone') as HTMLInputElement;
    sharing.click();
    await settle();
    buttonWithText(container, 'Save').click();
    await vi.waitFor(() => expect(mocks.updateSettings).toHaveBeenCalledOnce());

    expect(mocks.updateSettings).toHaveBeenCalledWith({ shareTimezone: true });
  });

  it('warns when an older server cannot keep a stored timezone private', async () => {
    mocks.currentUser.user = currentUser({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
    });
    mocks.currentUser.loading = false;

    const { container } = render(PreferencesPage);
    await settle();

    expect(container.querySelector('#share-timezone')).toBeNull();
    expect(container.textContent).toContain('This server version shares every stored time zone');
  });
});
