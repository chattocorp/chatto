import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import UserAvatarTestHarness from './UserAvatarTestHarness.svelte';

describe('UserAvatar', () => {
  it('shows a presence ring by default on medium avatars', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md' });
    const avatar = q(container, '[aria-label="alice"]')!;

    expect(avatar.className).toContain('ring-1');
    expect(avatar.className).toContain('ring-green-500');
    expect(q(container, '[aria-label="Online"]')).toBeTruthy();
    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeFalsy();
  });

  it('shows custom status badges when requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showStatus: true });

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeTruthy();
  });

  it('does not show presence rings on small avatars', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showPresence: true });
    const avatar = q(container, '[aria-label="alice"]')!;

    expect(avatar.className).not.toContain('ring-1');
    expect(avatar.className).not.toContain('ring-green-500');
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('allows presence rings to be disabled', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md', showPresence: false });
    const avatar = q(container, '[aria-label="alice"]')!;

    expect(avatar.className).not.toContain('ring-1');
    expect(avatar.className).not.toContain('ring-green-500');
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });
});
