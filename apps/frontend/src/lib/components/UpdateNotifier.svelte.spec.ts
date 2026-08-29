import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
	updated: { current: true },
	toastInfo: vi.fn(),
	forceReconnect: vi.fn(),
	onNavigate: vi.fn()
}));

vi.mock('$app/state', () => ({
	updated: mocks.updated
}));

vi.mock('$app/navigation', () => ({
	onNavigate: mocks.onNavigate
}));

vi.mock('$lib/i18n/messages', () => ({
	m: (key: string) => key
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
	serverConnectionManager: {
		originClient: {
			forceReconnect: mocks.forceReconnect
		}
	}
}));

vi.mock('$lib/ui/toast', () => ({
	toast: {
		info: mocks.toastInfo
	}
}));

import UpdateNotifier from './UpdateNotifier.svelte';

describe('UpdateNotifier', () => {
	beforeEach(() => {
		mocks.updated.current = true;
		mocks.toastInfo.mockReset();
		mocks.forceReconnect.mockReset();
		mocks.onNavigate.mockReset();
	});

	it('waits for the user to choose Reload after an update', async () => {
		const reloadApp = vi.fn();

		render(UpdateNotifier, { props: { reloadApp } });
		await tick();

		expect(reloadApp).not.toHaveBeenCalled();
		expect(mocks.onNavigate).not.toHaveBeenCalled();
		expect(mocks.toastInfo).toHaveBeenCalledOnce();
		expect(mocks.toastInfo).toHaveBeenCalledWith('ui.update_available', 0, {
			label: 'ui.reload',
			onClick: reloadApp
		});
		expect(mocks.forceReconnect).toHaveBeenCalledOnce();
		expect(mocks.forceReconnect).toHaveBeenCalledWith('app update detected');

		const action = mocks.toastInfo.mock.calls[0]?.[2] as { onClick: () => void };
		action.onClick();

		expect(reloadApp).toHaveBeenCalledOnce();
	});
});
