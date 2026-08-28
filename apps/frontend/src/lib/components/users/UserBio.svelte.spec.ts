import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { q } from '$lib/test-utils';
import UserBio from './UserBio.svelte';

describe('UserBio', () => {
	it('renders supported Markdown through the shared safe boundary', async () => {
		const { container } = render(UserBio, {
			props: { bio: '**Builds useful bots.** Visit [Chatto](https://chatto.dev).' }
		});

		await vi.waitFor(
			() => {
				expect(q(container, 'strong')?.textContent).toBe('Builds useful bots.');
				expect(q(container, 'a')?.getAttribute('href')).toBe('https://chatto.dev');
			},
			{ timeout: 5_000 }
		);
	});

	it('does not render source HTML', async () => {
		const { container } = render(UserBio, {
			props: { bio: '<img src=x onerror=alert(1)> safe' }
		});

		await vi.waitFor(
			() => {
				expect(q(container, '[data-testid="user-bio"]')?.textContent).toContain('<img');
			},
			{ timeout: 5_000 }
		);
		expect(q(container, 'img')).toBeNull();
	});
});
