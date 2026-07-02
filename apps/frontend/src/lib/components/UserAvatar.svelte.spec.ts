import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import { PresenceStatus } from '$lib/render/types';
import { q } from '$lib/test-utils';
import UserAvatarTestHarness from './UserAvatarTestHarness.svelte';

function computedBackgroundColor(color: string): string {
  const element = document.createElement('span');
  element.style.backgroundColor = color;
  document.body.append(element);
  const computed = window.getComputedStyle(element).backgroundColor;
  element.remove();
  return computed;
}

describe('UserAvatar', () => {
  it('renders medium avatars as circles without presence by default', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md' });
    const avatar = q(container, '[aria-label="alice"]')!;

    expect(avatar.className).toContain('rounded-full');
    expect(avatar.className).not.toContain('ring-');
    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeFalsy();
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('shows custom status badges when requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showStatus: true });

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeTruthy();
  });

  it('does not show presence dots on small avatars by default', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm' });
    const avatar = q(container, '[aria-label="alice"]')!;

    expect(avatar.className).toContain('rounded-full');
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('shows presence dots on small avatars when explicitly requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showPresence: true });
    const presenceDot = q(container, '[aria-label="Online"] span')!;

    expect(presenceDot.className).toContain('bg-presence-online');
  });

  it('shows presence dots on medium avatars when explicitly requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md', showPresence: true });
    const presenceDot = q(container, '[aria-label="Online"] span')!;

    expect(presenceDot.className).toContain('bg-presence-online');
  });

  it('renders away presence dots in yellow', () => {
    const { container } = render(UserAvatarTestHarness, {
      size: 'md',
      showPresence: true,
      presenceStatus: PresenceStatus.Away
    });
    const presenceDot = q(container, '[aria-label="Away"] span')!;
    const yellow500 = window
      .getComputedStyle(document.documentElement)
      .getPropertyValue('--color-yellow-500')
      .trim();

    expect(presenceDot.className).toContain('bg-presence-away');
    expect(window.getComputedStyle(presenceDot).backgroundColor).toBe(
      computedBackgroundColor(yellow500)
    );
  });

  it('keeps extra-small avatars free of presence overlays', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'xs', showPresence: true });

    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });
});
