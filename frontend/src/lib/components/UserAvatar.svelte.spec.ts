import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import UserAvatarTestHarness from './UserAvatarTestHarness.svelte';

describe('UserAvatar', () => {
  it('hides custom status badges on small avatars', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm' });

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeFalsy();
  });

  it('shows custom status badges on medium avatars', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md' });

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeTruthy();
  });
});
