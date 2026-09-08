import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { BotWebhookDeliveryStatus } from '@chatto/api-types/api/v1/bots_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  beforeNavigate: vi.fn(),
  successToast: vi.fn(),
  errorToast: vi.fn(),
  api: {
    listOutboundWebhooks: vi.fn(),
    listWebhookFailures: vi.fn(),
    createOutboundWebhook: vi.fn(),
    updateOutboundWebhook: vi.fn(),
    revokeOutboundWebhook: vi.fn()
  }
}));
vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.successToast, error: mocks.errorToast }
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
function fill(container: ParentNode, selector: string, value: string) {
  const input = container.querySelector(selector) as HTMLInputElement;
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}
const first = { id: 'first', name: 'Runling', url: 'https://example.com/first', enabled: true };
const second = {
  id: 'second',
  name: 'Other tool',
  url: 'https://example.com/second',
  enabled: false
};

describe('outbound webhook settings', () => {
  beforeEach(async () => {
    vi.resetAllMocks();
    queryClient.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.api.listOutboundWebhooks.mockResolvedValue([]);
  });
  afterEach(() => queryClient.clear());

  it('creates an endpoint in a dialog, toasts, and reveals only its own secret on request', async () => {
    let resolve!: (result: unknown) => void;
    mocks.api.createOutboundWebhook.mockReturnValue(new Promise((done) => (resolve = done)));
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() => expect(button(container, 'Create webhook').disabled).toBe(false));
    expect(container.querySelector('input[type="url"]')).toBeNull();
    button(container, 'Create webhook').click();
    flushSync();
    const dialog = container.querySelector('dialog[open]')!;
    fill(dialog, '#bot-outbound-name', 'Runling');
    fill(dialog, 'input[type="url"]', first.url);
    fill(dialog, 'input[type="password"]', 'Bearer receiver-secret');
    button(dialog, 'Create webhook').click();
    flushSync();
    await vi.waitFor(() =>
      expect(mocks.api.createOutboundWebhook).toHaveBeenCalledWith({
        botUserId: 'bot',
        name: 'Runling',
        url: first.url,
        authorization: 'Bearer receiver-secret',
        enabled: true
      })
    );
    const cancel = vi.fn();
    mocks.beforeNavigate.mock.calls.at(-1)?.[0]({ cancel });
    expect(cancel).toHaveBeenCalledOnce();
    mocks.api.listOutboundWebhooks.mockResolvedValue([first, second]);
    resolve({ webhook: first, signingSecret: 'show-once-secret' });
    await vi.waitFor(() => expect(mocks.successToast).toHaveBeenCalledWith('Webhook created.'));
    expect(container.querySelector('dialog[open]')).toBeNull();
    expect(container.textContent).not.toContain('show-once-secret');
    expect(container.querySelectorAll('[data-testid="bot-outbound-webhooks"] button').length).toBe(
      7
    );
    button(container, 'Show signing secret').click();
    flushSync();
    expect(container.textContent).toContain('show-once-secret');
    button(container, 'Got it').click();
    flushSync();
    expect(container.textContent).not.toContain('show-once-secret');
  });

  it('pauses one endpoint without creating credentials or changing another endpoint', async () => {
    mocks.api.listOutboundWebhooks.mockResolvedValue([first, second]);
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() => expect(container.textContent).toContain('Other tool'));
    mocks.api.listOutboundWebhooks.mockResolvedValue([{ ...first, enabled: false }, second]);
    button(container, 'Pause').click();
    await vi.waitFor(() => expect(mocks.successToast).toHaveBeenCalledWith('Webhook paused.'));
    expect(mocks.api.updateOutboundWebhook).toHaveBeenCalledWith('bot', 'first', false);
    expect(mocks.api.createOutboundWebhook).not.toHaveBeenCalled();
    expect(mocks.api.revokeOutboundWebhook).not.toHaveBeenCalled();
    expect(container.textContent).toContain(second.url);
  });

  it('shows endpoint failures and revokes only the confirmed endpoint', async () => {
    const failed = {
      ...first,
      latestDelivery: {
        status: BotWebhookDeliveryStatus.FAILED,
        reason: 'http_error',
        attempts: 5,
        httpStatus: 503
      }
    };
    mocks.api.listOutboundWebhooks.mockResolvedValue([failed, second]);
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() =>
      expect(container.textContent).toContain('Last recorded delivery failure.')
    );
    expect(container.textContent).toContain('HTTP status: 503');
    button(container, 'Revoke webhook').click();
    flushSync();
    expect(mocks.api.revokeOutboundWebhook).not.toHaveBeenCalled();
    mocks.api.listOutboundWebhooks.mockResolvedValue([second]);
    button(container.querySelector('dialog[open]')!, 'Revoke webhook').click();
    await vi.waitFor(() => expect(mocks.successToast).toHaveBeenCalledWith('Webhook revoked.'));
    expect(mocks.api.revokeOutboundWebhook).toHaveBeenCalledWith('bot', 'first');
    expect(container.textContent).not.toContain(first.url);
    expect(container.textContent).toContain(second.url);
  });

  it('keeps creation input across refresh and clears it after cancellation', async () => {
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() => expect(button(container, 'Create webhook').disabled).toBe(false));
    button(container, 'Create webhook').click();
    flushSync();
    fill(container, 'input[type="url"]', 'https://changed.example/hook');
    fill(container, 'input[type="password"]', 'secret');
    await queryClient.invalidateQueries();
    expect((container.querySelector('input[type="url"]') as HTMLInputElement).value).toBe(
      'https://changed.example/hook'
    );
    button(container, 'Cancel').click();
    flushSync();
    expect(mocks.api.createOutboundWebhook).not.toHaveBeenCalled();
    button(container, 'Create webhook').click();
    flushSync();
    expect((container.querySelector('input[type="url"]') as HTMLInputElement).value).toBe('');
    expect((container.querySelector('input[type="password"]') as HTMLInputElement).value).toBe('');
  });

  it('enforces the collection limit while keeping existing endpoint actions available', async () => {
    mocks.api.listOutboundWebhooks.mockResolvedValue(
      Array.from({ length: 20 }, (_, i) => ({ ...first, id: String(i) }))
    );
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() => expect(container.textContent).toContain('limit of 20'));
    expect(button(container, 'Create webhook').disabled).toBe(true);
    expect(button(container, 'Pause').disabled).toBe(false);
  });
  it('loads scoped failure history on demand and follows its cursor', async () => {
    mocks.api.listOutboundWebhooks.mockResolvedValue([first]);
    mocks.api.listWebhookFailures
      .mockResolvedValueOnce({
        failures: [{ id: 'f1', attempts: 2, httpStatus: 503, reason: 'http_error' }],
        nextCursor: 'next'
      })
      .mockResolvedValueOnce({
        failures: [{ id: 'f2', attempts: 3, httpStatus: 0, reason: 'transport_error' }],
        nextCursor: ''
      });
    const { container } = render(BotOutboundWebhookSection, { botId: 'bot' });
    await vi.waitFor(() => expect(container.textContent).toContain(first.url));
    expect(mocks.api.listWebhookFailures).not.toHaveBeenCalled();
    button(container, 'Recent failures').click();
    await vi.waitFor(() =>
      expect(
        container.querySelector('[data-testid="webhook-failure-history"]')?.textContent
      ).toContain('HTTP status: 503')
    );
    expect(mocks.api.listWebhookFailures).toHaveBeenCalledWith(
      'bot',
      'first',
      '',
      expect.any(AbortSignal)
    );
    button(container, 'Load more').click();
    await vi.waitFor(() => expect(container.textContent).toContain('transport_error'));
    expect(mocks.api.listWebhookFailures).toHaveBeenLastCalledWith(
      'bot',
      'first',
      'next',
      expect.any(AbortSignal)
    );
  });
});
