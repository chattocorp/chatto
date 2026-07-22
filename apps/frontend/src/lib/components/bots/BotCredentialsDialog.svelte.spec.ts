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

  it('confirms issuance and displays the returned secret exactly in the show-once panel', async () => {
    const updated = bot('2026-07-22T13:00:00.000Z');
    mocks.rotateAPIKey.mockResolvedValue({ bot: updated, apiKey: 'cht_BK-secret' });
    const onupdated = vi.fn();
    const { container } = render(BotCredentialsDialog, {
      props: { bot: bot(), canRotate: true, onupdated, onclose: vi.fn() }
    });

    button(container, 'Generate API key').click();
    flushSync();
    const generateButtons = [...container.querySelectorAll('button')].filter((item) =>
      item.textContent?.includes('Generate API key')
    );
    generateButtons.at(-1)?.click();
    await settle();

    expect(mocks.rotateAPIKey).toHaveBeenCalledWith('bot-1');
    expect(container.querySelector('[data-testid="bot-api-key-secret"]')?.textContent).toBe(
      'cht_BK-secret'
    );
    expect(container.textContent).toContain('Chatto will not show it again');
    expect(onupdated).toHaveBeenCalledWith(updated);
  });

  it('does not offer issuance to administrators', () => {
    const { container } = render(BotCredentialsDialog, {
      props: {
        bot: bot('2026-07-22T13:00:00.000Z'),
        canRotate: false,
        onupdated: vi.fn(),
        onclose: vi.fn()
      }
    });

    expect(container.textContent).not.toContain('Rotate API key');
    expect(container.textContent).toContain('Revoke API key');
  });

  it('confirms revocation and returns the credential-free bot', async () => {
    const updated = bot();
    mocks.revokeAPIKey.mockResolvedValue(updated);
    const onupdated = vi.fn();
    const { container } = render(BotCredentialsDialog, {
      props: {
        bot: bot('2026-07-22T13:00:00.000Z'),
        canRotate: false,
        onupdated,
        onclose: vi.fn()
      }
    });

    button(container, 'Revoke API key').click();
    flushSync();
    const revokeButtons = [...container.querySelectorAll('button')].filter((item) =>
      item.textContent?.includes('Revoke API key')
    );
    revokeButtons.at(-1)?.click();
    await settle();

    expect(mocks.revokeAPIKey).toHaveBeenCalledWith('bot-1');
    expect(onupdated).toHaveBeenCalledWith(updated);
  });
});
