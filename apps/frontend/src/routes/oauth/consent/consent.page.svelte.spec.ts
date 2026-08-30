import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ConsentPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
	csrfFetch: vi.fn()
}));

vi.mock('$lib/auth/csrf', () => ({ csrfFetch: mocks.csrfFetch }));

function consentResponse(overrides: Record<string, unknown> = {}) {
	return {
		redirectUri: 'https://callback.example/oauth/callback',
		redirectOrigin: 'https://callback.example',
		clientId: 'https://client.example/oauth/metadata.json',
		clientName: 'Example Client',
		clientUri: 'https://client.example',
		resource: '',
		scopes: [],
		...overrides
	};
}

describe('OAuth consent client identity', () => {
	beforeEach(() => {
		mocks.csrfFetch.mockReset();
	});

	it('shows the exact MCP room scope instead of broad message access', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () =>
				new Response(
					JSON.stringify(
						consentResponse({
							resource: 'https://chat.example/mcp',
							scopes: ['chatto:rooms:read']
						})
					),
					{ status: 200 }
				)
			)
		);

		const { getByText } = render(ConsentPage);

		await expect.element(getByText('It can list the rooms that are available to you.')).toBeVisible();
		await expect.element(getByText('It can join and leave rooms as you.')).not.toBeInTheDocument();
		await expect.element(getByText('It can read and send messages as you.')).not.toBeInTheDocument();
	});

	it('shows every capability in the complete MCP grant', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () =>
				new Response(
					JSON.stringify(
						consentResponse({
							resource: 'https://chat.example/mcp',
							scopes: [
								'chatto:messages:read',
								'chatto:messages:write',
								'chatto:rooms:read',
								'chatto:rooms:write'
							]
						})
					),
					{ status: 200 }
				)
			)
		);

		const { getByText } = render(ConsentPage);

		await expect.element(getByText('It can list the rooms that are available to you.')).toBeVisible();
		await expect.element(getByText('It can join and leave rooms as you.')).toBeVisible();
		await expect.element(getByText('It can read and send messages as you.')).toBeVisible();
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

	it('warns about a local callback and does not promise remembered consent', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () =>
				new Response(
					JSON.stringify(
						consentResponse({
							redirectUri: 'http://tool.feature.localhost:6276/oauth/callback',
							redirectOrigin: 'http://tool.feature.localhost:6276',
						})
					),
					{ status: 200 }
				)
			)
		);

		const { getByText } = render(ConsentPage);

		await expect
			.element(
				getByText(
					'This app will receive access through a local address on this device. Continue only if you started this request. Callback: http://tool.feature.localhost:6276'
				)
			)
			.toBeVisible();
		await expect
			.element(getByText('Chatto will remember this approval for this address.'))
			.not.toBeInTheDocument();
	});
});
