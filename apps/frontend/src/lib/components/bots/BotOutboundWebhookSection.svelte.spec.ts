import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { BotWebhookDeliveryStatus } from '@chatto/api-types/api/v1/bots_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  beforeNavigate: vi.fn(),
  api: {
    getOutboundWebhook: vi.fn(),
    replaceOutboundWebhook: vi.fn(),
    deleteOutboundWebhook: vi.fn()
  }
}));
vi.mock('$app/navigation', async (original) => ({
  ...(await original<typeof import('$app/navigation')>()),
  beforeNavigate: mocks.beforeNavigate
}));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'webhook-test',
    connection: { queryScope: 'session', getAPI: () => mocks.api }
  })
}));
import BotOutboundWebhookSection from './BotOutboundWebhookSection.svelte';

function button(container: ParentNode, text: string) {
  const element = [...container.querySelectorAll('button')].find(
    (item) => item.textContent?.trim() === text
  );
  if (!element) throw new Error(`Missing button: ${text}`);
  return element;
}

describe('outbound webhook settings', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    queryClient.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.api.getOutboundWebhook.mockResolvedValue(null);
  });
  afterEach(() => queryClient.clear());

  it('protects a pending signing secret and clears submitted credentials', async () => {
    let resolve!: (result: { signingSecret: string }) => void;
    mocks.api.replaceOutboundWebhook.mockReturnValue(new Promise((done) => (resolve = done)));
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() => expect(button(container, 'Save endpoint').disabled).toBe(false));
    const url = container.querySelector('input[type="url"]') as HTMLInputElement;
    const auth = container.querySelector('input[type="password"]') as HTMLInputElement;
    expect(url.labels?.[0]?.textContent).toContain('Destination URL');
    expect(auth.labels?.[0]?.textContent).toContain('Authorization header');
    url.value = 'https://example.com/secret';
    url.dispatchEvent(new Event('input', { bubbles: true }));
    auth.value = 'Bearer tool-secret';
    auth.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    button(container, 'Save endpoint').click();
    flushSync();
    await vi.waitFor(() =>
      expect(mocks.api.replaceOutboundWebhook).toHaveBeenCalledWith({
        botUserId: 'bot',
        url: 'https://example.com/secret',
        authorization: 'Bearer tool-secret',
        enabled: true
      })
    );
    const guard = mocks.beforeNavigate.mock.calls.at(-1)?.[0];
    const cancel = vi.fn();
    guard({ cancel });
    expect(cancel).toHaveBeenCalledOnce();
    resolve({ signingSecret: 'show-once-signing-secret' });
    await vi.waitFor(() => expect(container.textContent).toContain('show-once-signing-secret'));
    expect(url.value).toBe('');
    expect(auth.value).toBe('');
    button(container, 'Got it').click();
    flushSync();
    expect(container.textContent).not.toContain('show-once-signing-secret');
  });

  it('shows durable failure and removes an endpoint only after confirmation', async () => {
    mocks.api.getOutboundWebhook.mockResolvedValue({
      id: 'endpoint',
      enabled: true,
      latestDelivery: {
        status: BotWebhookDeliveryStatus.FAILED,
        reason: 'http_error',
        attempts: 5,
        httpStatus: 503
      }
    });
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() => expect(container.textContent).toContain('Latest delivery failed.'));
    expect(container.textContent).toContain('Attempts: 5');
    expect(container.textContent).toContain('HTTP status: 503');
    button(container, 'Remove endpoint').click();
    flushSync();
    expect(mocks.api.deleteOutboundWebhook).not.toHaveBeenCalled();
    mocks.api.getOutboundWebhook.mockResolvedValue(null);
    button(container, 'Confirm').click();
    await vi.waitFor(() => expect(mocks.api.deleteOutboundWebhook).toHaveBeenCalledWith('bot'));
  });
});
