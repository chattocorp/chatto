import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import UserAvatarTestHarness from './UserAvatarTestHarness.svelte';

describe('UserAvatar', () => {
  it('hides presence and custom status badges by default', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md' });

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeFalsy();
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('shows custom status badges when requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showStatus: true });

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeTruthy();
  });

  it('shows presence badges when requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showPresence: true });

    expect(q(container, '[aria-label="Online"]')).toBeTruthy();
  });
});
