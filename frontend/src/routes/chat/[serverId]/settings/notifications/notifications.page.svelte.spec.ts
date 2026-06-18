import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import NotificationsPage from './+page.svelte';
import { NotificationLevel } from '$lib/gql/graphql';
import { q } from '$lib/test-utils';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import { defaultNotificationSoundFilters } from '$lib/audio/notificationSounds';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutation: vi.fn(),
  playNotificationSound: vi.fn(),
  notificationLevels: {
    setServerPreference: vi.fn(),
    setRoomPreference: vi.fn()
  }
}));

vi.mock('$lib/audio/notificationSounds', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/audio/notificationSounds')>();
  return {
    ...actual,
    playNotificationSound: mocks.playNotificationSound
  };
});

vi.mock('$lib/notifications/pushNotifications', () => ({
  isSupported: () => true,
  isSubscribed: vi.fn().mockResolvedValue(false),
  subscribe: vi.fn().mockResolvedValue(true),
  unsubscribe: vi.fn().mockResolvedValue(true)
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      serverInfo: {
        pushNotificationsEnabled: false,
        vapidPublicKey: null
      },
      notificationLevels: mocks.notificationLevels
    })
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: mocks.query,
      mutation: mocks.mutation,
      subscription: vi.fn()
    }
  })
}));

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function setRangeValue(input: HTMLInputElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

function commitRangeValue(input: HTMLInputElement, value: string) {
  setRangeValue(input, value);
  input.dispatchEvent(new Event('change', { bubbles: true }));
  flushSync();
}

function preferenceResult() {
  return {
    server: {
      viewerNotificationPreference: {
        level: NotificationLevel.Normal,
        effectiveLevel: NotificationLevel.Normal
      }
    },
    viewer: {
      user: {
        rooms: [
          {
            id: 'room-1',
            name: 'general',
            viewerNotificationPreference: {
              level: NotificationLevel.Default,
              effectiveLevel: NotificationLevel.Normal
            }
          }
        ]
      }
    }
  };
}

function buttonWithText(container: Element, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll('button')).find((candidate) =>
    candidate.textContent?.includes(text)
  );
  if (!button) {
    throw new Error(`Button with text "${text}" not found`);
  }
  return button;
}

describe('Notification settings page', () => {
  beforeEach(() => {
    localStorage.clear();
    userPreferences.notificationSound = 'chime-up';
    userPreferences.resetNotificationSoundFilters();
    mocks.playNotificationSound.mockClear();
    mocks.notificationLevels.setServerPreference.mockClear();
    mocks.notificationLevels.setRoomPreference.mockClear();
    mocks.query.mockReset();
    mocks.query.mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({
        data: preferenceResult(),
        error: null
      })
    });
    mocks.mutation.mockReset();
  });

  it('renders notification levels and sound choices from mocked state', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    await expect.element(q(container, 'h1')).toHaveTextContent('Notifications');
    await expect
      .element(q(container, '[data-testid="room-notification-general"]'))
      .toBeInTheDocument();
    expect(container.textContent).toContain('Notification Sound');
    expect(container.textContent).toContain('Silent');
    expect(container.textContent).toContain('Simple');
    expect(container.textContent).toContain('Soft Pop');
    expect(container.textContent).toContain('Sound Shape');
    await expect.element(q(container, '[data-testid="notification-volume-filter"]')).toBeVisible();
    await expect
      .element(q(container, '[data-testid="notification-high-pass-filter"]'))
      .toBeVisible();
    await expect
      .element(q(container, '[data-testid="notification-low-pass-filter"]'))
      .toBeVisible();
    await expect.element(q(container, '[data-testid="notification-echo-filter"]')).toBeVisible();
    await expect.element(q(container, '[data-testid="notification-reverb-filter"]')).toBeVisible();
    await expect.element(q(container, '[data-testid="notification-crunch-filter"]')).toBeVisible();
    expect(container.querySelector('.uil--volume')).not.toBeNull();
    expect(container.querySelector('.uil--bolt')).not.toBeNull();
    expect(container.querySelector('.uil--volume-mute')).not.toBeNull();
    expect(container.querySelector('.uil--redo')).not.toBeNull();
    expect(container.querySelector('.uil--cloud')).not.toBeNull();
    expect(container.querySelector('.uil--fire')).not.toBeNull();
  });

  it('selects and persists a non-silent notification sound', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const softPopButton = buttonWithText(container, 'Soft Pop');
    softPopButton.click();
    flushSync();

    expect(userPreferences.notificationSound).toBe('pop');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      notificationSound: 'pop'
    });
    expect(mocks.playNotificationSound).toHaveBeenCalledWith(
      'pop',
      defaultNotificationSoundFilters
    );
    await expect.element(softPopButton).toHaveClass(/border-accent/);
  });

  it('selects silent mode without previewing a sound', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const silentButton = buttonWithText(container, 'Silent');
    silentButton.click();
    flushSync();

    expect(userPreferences.notificationSound).toBe('silent');
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
    await expect.element(silentButton).toHaveClass(/border-accent/);
  });

  it('updates and persists notification sound filter sliders', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    setRangeValue(
      q(container, '[data-testid="notification-volume-filter"]') as HTMLInputElement,
      '1.5'
    );
    setRangeValue(
      q(container, '[data-testid="notification-high-pass-filter"]') as HTMLInputElement,
      '500'
    );
    setRangeValue(
      q(container, '[data-testid="notification-low-pass-filter"]') as HTMLInputElement,
      '63'
    );
    setRangeValue(
      q(container, '[data-testid="notification-echo-filter"]') as HTMLInputElement,
      '35'
    );
    setRangeValue(
      q(container, '[data-testid="notification-reverb-filter"]') as HTMLInputElement,
      '45'
    );
    setRangeValue(
      q(container, '[data-testid="notification-crunch-filter"]') as HTMLInputElement,
      '55'
    );

    expect(userPreferences.notificationSoundFilters).toEqual({
      volume: 1.5,
      highPassHz: 500,
      lowPassHz: 7904,
      echo: 35,
      reverb: 45,
      crunch: 55
    });
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      notificationSoundFilters: {
        volume: 1.5,
        highPassHz: 500,
        lowPassHz: 7904,
        echo: 35,
        reverb: 45,
        crunch: 55
      }
    });
    expect(container.textContent).toContain('150%');
    expect(container.textContent).toContain('Tinny');
    expect(container.textContent).toContain('24%');
    expect(container.textContent).toContain('Muffled');
    expect(container.textContent).toContain('63%');
    expect(container.textContent).toContain('Echo');
    expect(container.textContent).toContain('35%');
    expect(container.textContent).toContain('Reverb');
    expect(container.textContent).toContain('45%');
    expect(container.textContent).toContain('Crunch');
    expect(container.textContent).toContain('55%');
  });

  it('previews the selected sound with the current filters', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    setRangeValue(
      q(container, '[data-testid="notification-high-pass-filter"]') as HTMLInputElement,
      '400'
    );
    mocks.playNotificationSound.mockClear();

    buttonWithText(container, 'Preview').click();
    flushSync();

    expect(mocks.playNotificationSound).toHaveBeenCalledWith('chime-up', {
      ...defaultNotificationSoundFilters,
      highPassHz: 400
    });
  });

  it('previews a filter change only when the slider change is committed', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    const volumeInput = q(
      container,
      '[data-testid="notification-volume-filter"]'
    ) as HTMLInputElement;
    mocks.playNotificationSound.mockClear();

    setRangeValue(volumeInput, '1.25');
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();

    volumeInput.dispatchEvent(new Event('change', { bubbles: true }));
    flushSync();

    expect(mocks.playNotificationSound).toHaveBeenCalledOnce();
    expect(mocks.playNotificationSound).toHaveBeenCalledWith('chime-up', {
      ...defaultNotificationSoundFilters,
      volume: 1.25
    });
  });

  it('does not preview committed filter changes while Silent is selected', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'Silent').click();
    flushSync();
    mocks.playNotificationSound.mockClear();

    commitRangeValue(
      q(container, '[data-testid="notification-echo-filter"]') as HTMLInputElement,
      '60'
    );

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('disables preview when silent is selected', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    buttonWithText(container, 'Silent').click();
    flushSync();
    mocks.playNotificationSound.mockClear();

    const previewButton = buttonWithText(container, 'Preview');
    expect(previewButton.disabled).toBe(true);
    previewButton.click();
    flushSync();

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('resets notification sound filters to defaults', async () => {
    const { container } = render(NotificationsPage);
    await settle();

    setRangeValue(
      q(container, '[data-testid="notification-volume-filter"]') as HTMLInputElement,
      '0.5'
    );
    buttonWithText(container, 'Reset').click();
    flushSync();

    expect(userPreferences.notificationSoundFilters).toEqual(defaultNotificationSoundFilters);
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      notificationSoundFilters: defaultNotificationSoundFilters
    });
    expect(container.textContent).toContain('100%');
  });
});
