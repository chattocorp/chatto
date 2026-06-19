import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import {
	GetViewerResponse,
	ServerProfileView,
	Viewer,
	ViewerPermissionsView
} from '$lib/pb/chatto/api/v1/chat_pb';
import { User } from '$lib/pb/chatto/core/v1/models_pb';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => {
	const client = {
		getViewer: vi.fn()
	};

	return {
		mocks: {
			client,
			showConnectionLostIcon: false,
			server: {
				id: 'remote',
				url: 'https://remote.example.com',
				name: 'Remote Chatto',
				iconUrl: null,
				token: 'token',
				userId: 'user-1',
				userLogin: 'alice',
				userDisplayName: 'Alice',
				userAvatarUrl: null,
				addedAt: 0
			},
			store: {
				notifications: { fetch: vi.fn().mockResolvedValue(undefined) },
				roomUnread: {
					clear: vi.fn(),
					setServerHasUnread: vi.fn(),
					setRoomUnread: vi.fn(),
					getFirstUnreadRoomId: vi.fn().mockReturnValue(null)
				},
				notificationLevels: {
					setServerPreference: vi.fn(),
					setRoomPreference: vi.fn(),
					isRoomMuted: vi.fn().mockReturnValue(false),
					isServerMuted: vi.fn().mockReturnValue(false)
				},
				pendingHighlights: { set: vi.fn() },
				serverInfo: {
					name: 'Chatto',
					iconUrl: null
				},
				rooms: { refresh: vi.fn().mockResolvedValue(undefined) },
				setPermissions: vi.fn(),
				serverIndicator: vi.fn().mockReturnValue(null)
			}
		}
	};
});

vi.mock('$app/state', () => ({
	page: {
		params: {
			serverId: 'other-server',
			roomId: undefined
		}
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$app/paths', () => ({
	resolve: (path: string, params?: Record<string, string>) =>
		path
			.replace('[serverId]', params?.serverId ?? '')
			.replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('$lib/hooks', () => ({
	useTabResumeCallback: (callback: () => void) => {
		void callback();
	}
}));

vi.mock('$lib/eventBus.svelte', () => ({
	createEventBusHandlerRegistrar: vi.fn(() => undefined)
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
	serverConnectionManager: {
		getClient: vi.fn(() => ({
			get showConnectionLostIcon() {
				return mocks.showConnectionLostIcon;
			}
		}))
	}
}));

vi.mock('$lib/state/server/wireEventBus.svelte', () => ({
	wireEventBusManager: {
		getClient: vi.fn(() => mocks.client)
	}
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
	serverRegistry: {
		isOriginServer: vi.fn(() => false),
		getServer: vi.fn(() => mocks.server),
		getStore: vi.fn(() => mocks.store)
	}
}));

import ServerSidebarEntry from './ServerSidebarEntry.svelte';

describe('ServerSidebarEntry', () => {
	let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

	beforeEach(() => {
		consoleErrorSpy?.mockRestore();
		consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
		mocks.showConnectionLostIcon = false;
		mocks.client.getViewer.mockReset();
		mocks.store.notifications.fetch.mockClear();
		mocks.store.rooms.refresh.mockClear();
		mocks.store.serverIndicator.mockReturnValue(null);
		mocks.store.serverInfo.name = 'Chatto';
		mocks.store.serverInfo.iconUrl = null;
	});

	afterEach(() => {
		consoleErrorSpy.mockRestore();
	});

	it('keeps a failed server in the gutter as a dimmed icon', async () => {
		mocks.client.getViewer.mockRejectedValue(new Error('connection refused'));

		const { container } = render(ServerSidebarEntry, {
			props: {
				serverId: 'remote',
				currentUserId: 'user-1'
			}
		});

		await vi.waitFor(() => {
			expect(mocks.client.getViewer).toHaveBeenCalled();
		});

		const icon = q(container, '[data-testid="server-icon"]');
		await expect.element(icon).toBeInTheDocument();
		await expect.element(icon).toHaveClass('opacity-40');
		await expect.element(icon).toHaveAttribute(
			'title',
			'Remote Chatto (connection unavailable)'
		);
		expect(container.textContent).toContain('R');
	});

	it('removes the dimmed state after sidebar init succeeds', async () => {
		mocks.client.getViewer.mockResolvedValue(
			new GetViewerResponse({
				serverProfile: new ServerProfileView({
					name: 'Loaded Remote',
					logoUrl: ''
				}),
				viewer: new Viewer({
					user: new User({
						id: 'user-1',
						login: 'alice',
						displayName: 'Alice'
					}),
					permissions: new ViewerPermissionsView({
						canStartDms: true
					})
				})
			})
		);

		const { container } = render(ServerSidebarEntry, {
			props: {
				serverId: 'remote',
				currentUserId: 'user-1'
			}
		});

		const icon = q(container, '[data-testid="server-icon"]');
		await expect.element(icon).toBeInTheDocument();
		await expect.element(icon).not.toHaveClass('opacity-40');
		await expect.element(icon).toHaveAttribute('title', 'Loaded Remote');
		expect(container.textContent).toContain('L');
	});
});
