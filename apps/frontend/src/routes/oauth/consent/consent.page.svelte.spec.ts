import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ConsentPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
	csrfFetch: vi.fn()
}));

vi.mock('$lib/auth/csrf', () => ({ csrfFetch: mocks.csrfFetch }));

function consentResponse(overrides: Record<string, string> = {}) {
	return {
		redirectUri: 'https://callback.example/oauth/callback',
		redirectOrigin: 'https://callback.example',
		clientId: 'https://client.example/oauth/metadata.json',
		clientName: 'Example Client',
		clientUri: 'https://client.example',
		...overrides
	};
}

describe('OAuth consent client identity', () => {
	beforeEach(() => {
		mocks.csrfFetch.mockReset();
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('shows the validated client identity instead of the callback host', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () => new Response(JSON.stringify(consentResponse()), { status: 200 }))
		);

		const { getByText } = render(ConsentPage);

		await expect.element(getByText('Example Client')).toBeVisible();
		await expect.element(getByText('client.example')).toBeVisible();
		await expect.element(getByText('callback.example')).not.toBeInTheDocument();
	});

	it('allows a native private-scheme authorization request to be denied', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () =>
				new Response(
					JSON.stringify(
						consentResponse({
							redirectUri: 'com.example.chatto:/oauth/callback',
							redirectOrigin: 'com.example.chatto:',
							clientId: 'https://mobile.example/oauth/metadata.json',
							clientName: 'Example Mobile'
						})
					),
					{ status: 200 }
				)
			)
		);
		mocks.csrfFetch.mockResolvedValue(
			new Response(JSON.stringify({ error: 'expected test response' }), { status: 400 })
		);

		const { getByText, getByRole } = render(ConsentPage);

		await expect.element(getByText('Example Mobile')).toBeVisible();
		const deny = getByRole('button', { name: 'Cancel' });
		await expect.element(deny).toBeVisible();
		await deny.click();
		expect(mocks.csrfFetch).toHaveBeenCalledWith(
			'/oauth/consent/deny',
			expect.objectContaining({ method: 'POST' })
		);
	});
});
