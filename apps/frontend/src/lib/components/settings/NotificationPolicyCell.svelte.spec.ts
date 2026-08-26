import { beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { NotificationDeliveryMode } from '$lib/api-client/notifications';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import NotificationPolicyCell from './NotificationPolicyCell.svelte';

const baseProps = {
  field: 'directMessages' as const,
  causeLabel: 'Direct messages',
  scope: { kind: 'server' as const },
  scopeLabel: 'Test Server',
  effective: NotificationDeliveryMode.PUSH_NOTIFICATION,
  loading: false,
  disabled: false
};

describe('NotificationPolicyCell', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('cycles server defaults through Off → Badge → Notification → Push without Inherit', async () => {
    const onChange = vi.fn();
    const rendered = render(NotificationPolicyCell, {
      props: { ...baseProps, override: null, onChange }
    });

    const clickAndExpect = (expected: NotificationDeliveryMode | null) => {
      rendered.container.querySelector('button')!.click();
      flushSync();
      expect(onChange).toHaveBeenLastCalledWith(expected);
    };

    clickAndExpect(NotificationDeliveryMode.OFF);
    await rendered.rerender({
      ...baseProps,
      override: NotificationDeliveryMode.OFF,
      onChange
    });
    clickAndExpect(NotificationDeliveryMode.UNREAD_BADGE);
    await rendered.rerender({
      ...baseProps,
      override: NotificationDeliveryMode.UNREAD_BADGE,
      onChange
    });
    clickAndExpect(NotificationDeliveryMode.IN_APP_NOTIFICATION);
    await rendered.rerender({
      ...baseProps,
      override: NotificationDeliveryMode.IN_APP_NOTIFICATION,
      onChange
    });
    clickAndExpect(NotificationDeliveryMode.PUSH_NOTIFICATION);
    await rendered.rerender({
      ...baseProps,
      override: NotificationDeliveryMode.PUSH_NOTIFICATION,
      onChange
    });
    clickAndExpect(NotificationDeliveryMode.OFF);
  });

  it('cycles nested scopes through Inherit → Off → Badge → Notification → Push → Inherit', async () => {
    const onChange = vi.fn();
    const nestedProps = {
      ...baseProps,
      scope: { kind: 'roomGroup' as const, id: 'group-1' },
      scopeLabel: 'Lobby'
    };
    const rendered = render(NotificationPolicyCell, {
      props: { ...nestedProps, override: null, onChange }
    });

    const clickAndExpect = (expected: NotificationDeliveryMode | null) => {
      rendered.container.querySelector('button')!.click();
      flushSync();
      expect(onChange).toHaveBeenLastCalledWith(expected);
    };

    clickAndExpect(NotificationDeliveryMode.OFF);
    await rendered.rerender({
      ...nestedProps,
      override: NotificationDeliveryMode.OFF,
      onChange
    });
    clickAndExpect(NotificationDeliveryMode.UNREAD_BADGE);
    await rendered.rerender({
      ...nestedProps,
      override: NotificationDeliveryMode.UNREAD_BADGE,
      onChange
    });
    clickAndExpect(NotificationDeliveryMode.IN_APP_NOTIFICATION);
    await rendered.rerender({
      ...nestedProps,
      override: NotificationDeliveryMode.IN_APP_NOTIFICATION,
      onChange
    });
    clickAndExpect(NotificationDeliveryMode.PUSH_NOTIFICATION);
    await rendered.rerender({
      ...nestedProps,
      override: NotificationDeliveryMode.PUSH_NOTIFICATION,
      onChange
    });
    clickAndExpect(null);
  });

  it('supports keyboard activation and describes the current and next states', async () => {
    const onChange = vi.fn();
    const { container } = render(NotificationPolicyCell, {
      props: { ...baseProps, override: null, onChange }
    });
    const button = container.querySelector('button') as HTMLButtonElement;

    expect(button.ariaLabel).toContain('Direct messages');
    expect(button.ariaLabel).toContain('Test Server');
    expect(button.ariaLabel).toContain('Default: Push notification');
    expect(button.ariaLabel).not.toContain('Inherit');
    expect(button.ariaLabel).toContain('Activate to set Off');
    expect(button.hasAttribute('title')).toBe(false);

    button.focus();
    await userEvent.keyboard('{Enter}');
    expect(onChange).toHaveBeenCalledWith(NotificationDeliveryMode.OFF);
  });

  it('renders a server default at full intensity without an inheritance marker', () => {
    const { container } = render(NotificationPolicyCell, {
      props: { ...baseProps, override: null, onChange: vi.fn() }
    });

    expect(container.querySelector('[class~="icon-[ph--phone-fill]"]')).not.toBeNull();
    expect(container.querySelector('[class~="icon-[uil--link]"]')).toBeNull();
    expect(container.querySelector('[class~="text-warning"]')).not.toBeNull();
    expect(container.querySelector('[class~="bg-warning"]')).toBeNull();
    expect(container.querySelector('[class~="opacity-40"]')).toBeNull();
    expect(container.querySelector('[data-notification-source="default"]')).not.toBeNull();
  });

  it('renders Badge as a filled neutral bell', () => {
    const { container } = render(NotificationPolicyCell, {
      props: {
        ...baseProps,
        override: NotificationDeliveryMode.UNREAD_BADGE,
        effective: NotificationDeliveryMode.UNREAD_BADGE,
        onChange: vi.fn()
      }
    });

    expect(container.querySelector('[class~="icon-[ph--bell-fill]"]')).not.toBeNull();
    expect(container.querySelector('[class~="text-text"]')).not.toBeNull();
    expect(container.querySelector('[class~="text-warning"]')).toBeNull();
  });

  it('fades an inherited nested value without changing its icon', () => {
    const { container } = render(NotificationPolicyCell, {
      props: {
        ...baseProps,
        scope: { kind: 'room' as const, id: 'room-1' },
        override: null,
        onChange: vi.fn()
      }
    });

    expect(container.querySelector('[class~="icon-[ph--phone-fill]"]')).not.toBeNull();
    expect(container.querySelector('[class~="opacity-40"]')).not.toBeNull();
    expect(container.querySelector('[class~="icon-[uil--link]"]')).toBeNull();
    expect(container.querySelector('[data-notification-source="inherited"]')).not.toBeNull();
  });

  it('keeps keyboard focus while a save is pending', async () => {
    const onChange = vi.fn();
    const rendered = render(NotificationPolicyCell, {
      props: { ...baseProps, override: null, onChange }
    });
    const button = rendered.container.querySelector('button') as HTMLButtonElement;
    button.focus();

    await rendered.rerender({ ...baseProps, override: null, loading: true, onChange });
    expect(document.activeElement).toBe(button);
    expect(button.disabled).toBe(false);
    expect(button.getAttribute('aria-disabled')).toBe('true');
    button.click();
    expect(onChange).not.toHaveBeenCalled();

    await rendered.rerender({
      ...baseProps,
      override: NotificationDeliveryMode.OFF,
      onChange
    });
    expect(document.activeElement).toBe(button);
  });
});
