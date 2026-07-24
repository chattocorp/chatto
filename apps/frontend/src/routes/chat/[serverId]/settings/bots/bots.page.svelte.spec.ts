import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import BotsPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
	store: {
		serverInfo: {
			loading: true,
			supportsProtocolCapability: vi.fn(() => null as boolean | null)
		},
		permissions: { loaded: false },
		projection: { viewer: null as unknown }
	}
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
	getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
	serverRegistry: { getStore: () => mocks.store }
}));

vi.mock('$lib/api-client/viewer', () => ({
	viewerResponseToState: (viewer: unknown) => viewer
}));

describe('Bot settings page access gate', () => {
	beforeEach(() => {
		mocks.store.serverInfo.loading = true;
		mocks.store.serverInfo.supportsProtocolCapability.mockReset();
		mocks.store.serverInfo.supportsProtocolCapability.mockReturnValue(null);
		mocks.store.permissions.loaded = false;
		mocks.store.projection.viewer = null;
	});

	it('does not flash Access Denied while reload state is hydrating', () => {
		const { container } = render(BotsPage);
		flushSync();

		expect(container.textContent).not.toContain('Access Denied');
		expect(container.textContent).not.toContain(
			'You do not have permission to create or manage bots on this server.'
		);
	});

	it('shows Access Denied after loaded state confirms the viewer is not allowed', () => {
		mocks.store.serverInfo.loading = false;
		mocks.store.serverInfo.supportsProtocolCapability.mockReturnValue(true);
		mocks.store.permissions.loaded = true;
		mocks.store.projection.viewer = { viewerPermissions: {} };

		const { container } = render(BotsPage);
		flushSync();

		expect(container.textContent).toContain('Access Denied');
		expect(container.textContent).toContain(
			'You do not have permission to create or manage bots on this server.'
		);
	});
});
