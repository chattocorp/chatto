import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

const mocks = vi.hoisted(() => ({
	getBot: vi.fn(),
	batchGetUsers: vi.fn(),
	updateBot: vi.fn(),
	toastSuccess: vi.fn(),
	toastError: vi.fn(),
	settings: null as { timezone: string; timeFormat: TimeFormat } | null,
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
		store: {
			serverInfo: { supportsFeature: () => true },
			currentUser: { user: { settings: mocks.settings } }
		},
		connection: {
			queryScope: 'session-1',
			getAPI: () => ({
				getBot: mocks.getBot,
				batchGetUsers: mocks.batchGetUsers,
				updateBot: mocks.updateBot
			})
		},
		isCurrent: () => true
	})
}));

vi.mock('$lib/components/rbac', async () => ({
	UserPermissionsMatrix: (await import('./BotUserPermissionsMatrixMock.svelte')).default
}));

vi.mock('$lib/ui/toast', () => ({
	toast: { success: mocks.toastSuccess, error: mocks.toastError }
}));

import BotDetailPage from './+page.svelte';

function setInput(input: HTMLInputElement, value: string): void {
	input.value = value;
	input.dispatchEvent(new Event('input', { bubbles: true }));
	flushSync();
}

function buttonByText(root: ParentNode, text: string): HTMLButtonElement {
	const button = [...root.querySelectorAll('button')].find(
		(candidate) => candidate.textContent?.trim() === text
	);
	if (!(button instanceof HTMLButtonElement)) throw new Error(`Button not found: ${text}`);
	return button;
}

async function settle(): Promise<void> {
	await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
	flushSync();
}

describe('Bot detail page', () => {
	beforeEach(async () => {
		queryClient.clear();
		vi.clearAllMocks();
		mocks.settings = null;
		mocks.getBot.mockResolvedValue(mocks.bot);
		mocks.batchGetUsers.mockResolvedValue([]);
		mocks.updateBot.mockImplementation((input: { login?: string; displayName?: string }) =>
			Promise.resolve({
				...mocks.bot,
				login: input.login ?? mocks.bot.login,
				displayName: input.displayName ?? mocks.bot.displayName
			})
		);
		await loadLocaleMessages('en-GB');
		setReactiveLocale('en-GB');
	});

	it('shows the bot user ID and hydrates its owner as a reusable user identity', async () => {
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

	it('sends only the bot profile field changed in the edit dialog', async () => {
		const rendered = render(BotDetailPage);
		await settle();

		buttonByText(rendered.container, 'Edit').click();
		flushSync();
		setInput(
			rendered.container.querySelector('#edit-bot-login') as HTMLInputElement,
			'renamed_bot'
		);
		buttonByText(rendered.container, 'Save').click();
		await vi.waitFor(() => expect(mocks.updateBot).toHaveBeenCalledOnce());

		expect(mocks.updateBot).toHaveBeenCalledWith({
			botUserId: 'bot-user-id',
			login: 'renamed_bot'
		});
	});

	it('shows a friendly toast when OCC rejects a concurrent bot edit', async () => {
		mocks.updateBot.mockRejectedValue(new ConnectError('conflict', Code.Aborted));
		const rendered = render(BotDetailPage);
		await settle();

		buttonByText(rendered.container, 'Edit').click();
		flushSync();
		setInput(
			rendered.container.querySelector('#edit-bot-display-name') as HTMLInputElement,
			'Changed elsewhere'
		);
		buttonByText(rendered.container, 'Save').click();

		await vi.waitFor(() =>
			expect(mocks.toastError).toHaveBeenCalledWith(
				'Someone else updated this bot. Reload the page and try again.'
			)
		);
		expect(mocks.updateBot).toHaveBeenCalledWith({
			botUserId: 'bot-user-id',
			displayName: 'Changed elsewhere'
		});
	});

	it("formats API key timestamps with the viewer's timezone and time format", async () => {
		mocks.settings = {
			timezone: 'America/New_York',
			timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
		};
		const { container } = render(BotDetailPage);
		await settle();

		const expected = formatDateTime(
			mocks.bot.apiKeyCreatedAt,
			timeFormatSettingsFor(mocks.settings),
			'en-GB'
		);
		expect(container.textContent).toContain(expected);
	});
});
