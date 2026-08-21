import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
	getBot: vi.fn(),
	batchGetUsers: vi.fn(),
	bot: {
		id: 'bot-user-id',
		login: 'helper_bot',
		displayName: 'Helper Bot',
		avatarUrl: null,
		ownerUserId: 'owner-user-id',
		createdAt: null,
		apiKeyCreatedAt: new Date('2026-08-21T12:00:00Z'),
		apiKeyRotatedAt: null
	}
}));

vi.mock('$app/state', () => ({ page: { params: { botId: 'bot-user-id' } } }));

vi.mock('$lib/state/server/scope.svelte', () => ({
	useServerScope: () => ({
		serverId: 'server-1',
		store: { serverInfo: { supportsFeature: () => true } },
		connection: {
			queryScope: 'session-1',
			getAPI: () => ({ getBot: mocks.getBot, batchGetUsers: mocks.batchGetUsers })
		},
		isCurrent: () => true
	})
}));

vi.mock('$lib/components/rbac', async () => ({
	UserPermissionsMatrix: (await import('./BotUserPermissionsMatrixMock.svelte')).default
}));

import BotDetailPage from './+page.svelte';

describe('Bot detail page', () => {
	it('shows the bot user ID and hydrates its owner as a reusable user identity', async () => {
		queryClient.clear();
		mocks.getBot.mockResolvedValue(mocks.bot);
		mocks.batchGetUsers.mockResolvedValue([
			{
				id: 'owner-user-id',
				login: 'alice',
				displayName: 'Alice Owner',
				avatarUrl: null,
				deleted: false,
				isBot: false
			}
		]);
		const { container } = render(BotDetailPage);
		await vi.waitFor(() => {
			expect(container.textContent).toContain('Alice Owner');
		});

		expect(mocks.batchGetUsers).toHaveBeenCalledWith(['owner-user-id']);
		expect(container.textContent).toContain('User ID');
		expect(container.textContent).toContain('bot-user-id');
		expect(container.querySelector('button[title="Copy to clipboard"]')).not.toBeNull();
		expect(container.textContent).toContain('Owner');
		expect(container.textContent).toContain('Alice Owner');
		expect(container.textContent).not.toContain('owner-user-id');
		expect(container.querySelector('[data-testid="user-identity"]')).not.toBeNull();
	});
});
