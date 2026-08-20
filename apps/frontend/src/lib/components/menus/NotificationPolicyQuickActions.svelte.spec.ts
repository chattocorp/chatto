import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import {
  NotificationDeliveryMode,
  type NotificationPolicy,
  type NotificationPolicyOverrides
} from '$lib/api-client/notifications';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    getPolicy: vi.fn(),
    updatePolicy: vi.fn(),
    toastSuccess: vi.fn(),
    toastError: vi.fn()
  }
}));

vi.mock('$lib/ui/toast', () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError
  }
}));

import NotificationPolicyQuickActions from './NotificationPolicyQuickActions.svelte';

function modes<Value>(value: Value) {
  return {
    directMessages: value,
    directMentions: value,
    replies: value,
    roleMentions: value,
    hereMentions: value,
    allMentions: value,
    followedThreads: value,
    followedRooms: value,
    reactions: value
  };
}

function notificationPolicy(mode: NotificationDeliveryMode): NotificationPolicy {
  return {
    overrides: modes(mode) as NotificationPolicyOverrides,
    effective: modes(mode)
  };
}

describe('NotificationPolicyQuickActions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getPolicy.mockResolvedValue(notificationPolicy(NotificationDeliveryMode.ALERT));
    mocks.updatePolicy.mockResolvedValue(notificationPolicy(NotificationDeliveryMode.SILENT));
  });

  it('checks a uniform current preset and atomically updates every class for a room', async () => {
    mocks.getPolicy.mockResolvedValue({
      overrides: modes(null),
      effective: modes(NotificationDeliveryMode.ALERT)
    });
    const onupdated = vi.fn();
    const { container } = render(NotificationPolicyQuickActions, {
      props: {
        notificationStore: {
          getPolicy: mocks.getPolicy,
          updatePolicy: mocks.updatePolicy
        },
        roomId: 'room-1',
        onupdated
      }
    });

    const alert = await vi.waitFor(() => {
      const button = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
        (candidate) => candidate.textContent?.trim() === 'Alert'
      );
      expect(button).toBeDefined();
      expect(button).toHaveAttribute('aria-checked', 'true');
      return button!;
    });
    await expect.element(alert).toBeEnabled();

    const silent = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
      (candidate) => candidate.textContent?.trim() === 'Silent'
    );
    silent?.click();

    await vi.waitFor(() => {
      expect(mocks.updatePolicy).toHaveBeenCalledWith(
        modes(NotificationDeliveryMode.SILENT),
        'room-1'
      );
      expect(mocks.toastSuccess).toHaveBeenCalledWith('Saved!');
      expect(onupdated).toHaveBeenCalledOnce();
    });
  });

  it('keeps presets usable when the current policy cannot be loaded', async () => {
    mocks.getPolicy.mockRejectedValue(new Error('unavailable'));
    mocks.updatePolicy.mockRejectedValue(new Error('save rejected'));
    const { container } = render(NotificationPolicyQuickActions, {
      props: {
        notificationStore: {
          getPolicy: mocks.getPolicy,
          updatePolicy: mocks.updatePolicy
        }
      }
    });

    await vi.waitFor(() => expect(mocks.getPolicy).toHaveBeenCalledOnce());
    await new Promise((resolve) => setTimeout(resolve, 0));
    const off = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
      (candidate) => candidate.textContent?.trim() === 'Off'
    );
    expect(off).toBeDefined();
    off!.click();

    await vi.waitFor(() => {
      expect(mocks.updatePolicy).toHaveBeenCalledWith(modes(NotificationDeliveryMode.OFF), undefined);
      expect(mocks.toastError).toHaveBeenCalledWith('Failed to save notification policy');
    });
    expect(q(container, 'button[aria-checked="true"]')).toBeNull();
  });
});
