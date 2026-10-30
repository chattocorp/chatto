import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import type { CurrentUserState, CurrentUser } from '@chatto/client/auth/currentUser';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { q } from '$lib/test-utils';

const mocks = vi.hoisted(() => ({
  currentUser: null as unknown as CurrentUserState,
  updateSettings: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

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
    mocks.currentUser = createTestServerScope({ viewer: null, api: mocks }).currentUser;
    mocks.currentUser.loading = true;
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
    await expect.element(buttonWithText(container, 'Save')).toBeDisabled();
  });

  it('updates untouched values while preserving edits when saved settings change', async () => {
    mocks.currentUser.accept(
      currentUser({
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TIME_FORMAT_AUTO,
        shareTimezone: false
      })
    );
    const { container } = render(PreferencesPage);
    await settle();
    buttonWithText(container, '24-hour').click();
    mocks.currentUser.update('user-1', () => ({
      settings: {
        timezone: 'Pacific/Honolulu',
        timeFormat: TimeFormat.TIME_FORMAT_12_HOUR,
        shareTimezone: true
      }
    }));
    await settle();
    await expect
      .element(q(container, '[data-testid="timezone-input"]'))
      .toHaveValue('Pacific/Honolulu');
    await expect.element(q(container, '#share-timezone')).toBeChecked();
    await expect
      .element(buttonWithText(container, '24-hour'))
      .toHaveAttribute('aria-checked', 'true');
    mocks.updateSettings.mockResolvedValue({
      timezone: 'Pacific/Honolulu',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
      shareTimezone: true
    });
    buttonWithText(container, 'Save').click();
    await vi.waitFor(() =>
      expect(mocks.updateSettings).toHaveBeenCalledExactlyOnceWith({
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
      })
    );
    await expect.element(buttonWithText(container, 'Save')).toBeDisabled();
  });

  it('keeps edits after a failed save and clears them after a successful retry', async () => {
    const settings = {
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_AUTO,
      shareTimezone: false
    };
    mocks.currentUser.accept(currentUser(settings));
    mocks.updateSettings
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ ...settings, shareTimezone: true });
    const { container } = render(PreferencesPage);
    await settle();
    (q(container, '#share-timezone') as HTMLInputElement).click();
    await settle();
    buttonWithText(container, 'Save').click();
    await vi.waitFor(() => expect(container.textContent).toContain('offline'));
    await expect.element(q(container, '#share-timezone')).toBeChecked();
    await expect.element(buttonWithText(container, 'Save')).toBeEnabled();
    buttonWithText(container, 'Save').click();
    await vi.waitFor(() => expect(mocks.updateSettings).toHaveBeenCalledTimes(2));
    await expect.element(buttonWithText(container, 'Save')).toBeDisabled();
    mocks.currentUser.update('user-1', () => ({ settings }));
    await settle();
    await expect.element(q(container, '#share-timezone')).not.toBeChecked();
    await expect.element(buttonWithText(container, 'Save')).toBeDisabled();
  });

  it('preserves edits made while a save is pending', async () => {
    const settings = {
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_AUTO,
      shareTimezone: false
    };
    mocks.currentUser.accept(currentUser(settings));
    let finish!: (settings: NonNullable<CurrentUser['settings']>) => void;
    mocks.updateSettings.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    const { container } = render(PreferencesPage);
    await settle();
    (q(container, '#share-timezone') as HTMLInputElement).click();
    await settle();
    buttonWithText(container, 'Save').click();
    await vi.waitFor(() => expect(mocks.updateSettings).toHaveBeenCalledOnce());
    buttonWithText(container, '24-hour').click();
    finish({ ...settings, shareTimezone: true });
    await settle();
    await expect
      .element(buttonWithText(container, '24-hour'))
      .toHaveAttribute('aria-checked', 'true');
    await expect.element(buttonWithText(container, 'Save')).toBeEnabled();
    mocks.updateSettings.mockResolvedValue({
      ...settings,
      shareTimezone: true,
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
    });
    buttonWithText(container, 'Save').click();
    await vi.waitFor(() =>
      expect(mocks.updateSettings).toHaveBeenLastCalledWith({
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
      })
    );
    await expect.element(buttonWithText(container, 'Save')).toBeDisabled();
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

  it('blocks invalid timezone text and sends an explicit clear as null', async () => {
    const settings = {
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_AUTO,
      shareTimezone: false
    };
    mocks.currentUser.accept(currentUser(settings));
    mocks.updateSettings.mockResolvedValue({ ...settings, timezone: null });
    const { container } = render(PreferencesPage);
    const input = q(container, '[data-testid="timezone-input"]') as HTMLInputElement;
    input.value = 'invalid zone';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    await expect.element(buttonWithText(container, 'Save')).toBeDisabled();
    expect(mocks.updateSettings).not.toHaveBeenCalled();
    input.value = '';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    buttonWithText(container, 'Save').click();
    await vi.waitFor(() =>
      expect(mocks.updateSettings).toHaveBeenCalledExactlyOnceWith({ timezone: null })
    );
    await expect.element(input).toHaveValue('');
    await expect.element(buttonWithText(container, 'Save')).toBeDisabled();
  });
});
