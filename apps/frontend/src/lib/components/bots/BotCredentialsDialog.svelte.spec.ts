import '../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import type { BotAccount } from '$lib/api-client/bots';
import BotCredentialsDialog from './BotCredentialsDialog.svelte';

const mocks = vi.hoisted(() => ({
	rotateAPIKey: vi.fn(),
	revokeAPIKey: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
	useConnection: () => () => ({ connectBaseUrl: '/api/connect', bearerToken: 'token' })
}));

vi.mock('$lib/api-client/bots', () => ({
	createBotAPI: () => ({
		rotateAPIKey: mocks.rotateAPIKey,
		revokeAPIKey: mocks.revokeAPIKey
	})
}));

function bot(apiKeyCreatedAt: string | null = null): BotAccount {
	return {
		id: 'bot-1',
		login: 'helper_bot',
		displayName: 'Helper Bot',
		avatarUrl: null,
		ownerId: 'owner-1',
		description: 'Helps people',
		createdAt: '2026-07-22T12:00:00.000Z',
		apiKeyCreatedAt
	};
}

async function settle() {
	await Promise.resolve();
	await Promise.resolve();
	flushSync();
}

function button(container: HTMLElement, label: string): HTMLButtonElement {
	const found = [...container.querySelectorAll('button')].find((item) =>
		item.textContent?.includes(label)
	);
	if (!found) throw new Error(`button not found: ${label}`);
	return found;
}

describe('BotCredentialsDialog', () => {
	beforeEach(() => {
		mocks.rotateAPIKey.mockReset();
		mocks.revokeAPIKey.mockReset();
	});

	it('shows a creation secret directly without a confirmation dialog', () => {
		const { container } = render(BotCredentialsDialog, {
			props: {
				bot: bot('2026-07-22T13:00:00.000Z'),
				action: 'show',
				initialSecret: 'cht_BK-created-secret',
				onupdated: vi.fn(),
				onclose: vi.fn()
			}
		});

		expect(container.querySelectorAll('dialog[open]')).toHaveLength(1);
		expect(container.querySelector('[data-testid="bot-api-key-secret"]')?.textContent).toBe(
			'cht_BK-created-secret'
		);
		expect(container.textContent).not.toContain('Reset API key?');
	});

	it('replaces the confirmation with the show-once dialog after rotation', async () => {
		const updated = bot('2026-07-22T13:00:00.000Z');
		mocks.rotateAPIKey.mockResolvedValue({ bot: updated, apiKey: 'cht_BK-rotated-secret' });
		const onupdated = vi.fn();
		const { container } = render(BotCredentialsDialog, {
			props: {
				bot: bot('2026-07-22T12:00:00.000Z'),
				action: 'rotate',
				onupdated,
				onclose: vi.fn()
			}
		});

		expect(container.querySelectorAll('dialog[open]')).toHaveLength(1);
		expect(container.textContent).toContain('Reset API key?');
		button(container, 'Reset API key').click();
		await settle();

		expect(mocks.rotateAPIKey).toHaveBeenCalledWith('bot-1');
		expect(container.querySelectorAll('dialog[open]')).toHaveLength(1);
		expect(container.textContent).not.toContain('Reset API key?');
		expect(container.querySelector('[data-testid="bot-api-key-secret"]')?.textContent).toBe(
			'cht_BK-rotated-secret'
		);
		expect(onupdated).toHaveBeenCalledWith(updated);
	});

	it('confirms revocation and closes without a second dialog', async () => {
		const updated = bot();
		mocks.revokeAPIKey.mockResolvedValue(updated);
		const onupdated = vi.fn();
		const onclose = vi.fn();
		const { container } = render(BotCredentialsDialog, {
			props: {
				bot: bot('2026-07-22T13:00:00.000Z'),
				action: 'revoke',
				onupdated,
				onclose
			}
		});

		button(container, 'Revoke API key').click();
		await settle();

		expect(mocks.revokeAPIKey).toHaveBeenCalledWith('bot-1');
		expect(onupdated).toHaveBeenCalledWith(updated);
		expect(onclose).toHaveBeenCalledOnce();
	});
});
