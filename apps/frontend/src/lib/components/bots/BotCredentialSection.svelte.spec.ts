import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';

const mocks = vi.hoisted(() => ({
  beforeNavigate: vi.fn()
}));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  beforeNavigate: mocks.beforeNavigate
}));

import BotCredentialSection from './BotCredentialSection.svelte';

const labels = {
  title: 'Credentials',
  description: 'Manage credentials.',
  create: 'Create credential',
  name: 'Credential name',
  createdAt: 'Created',
  lastUsed: 'Last used',
  empty: 'No credentials.',
  limitReached: 'Credential limit reached.',
  revoke: 'Revoke credential',
  revokeWarning: 'This credential will stop working.',
  issuedTitle: 'Save credential',
  issuedWarning: 'This credential is shown once.',
  copied: 'Copied'
};

const item = {
  id: 'credential-1',
  name: 'Production',
  createdAt: '30 August 2026',
  lastUsed: 'No use recorded'
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function buttonByText(root: ParentNode, text: string): HTMLButtonElement {
  const button = [...root.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === text
  );
  if (!(button instanceof HTMLButtonElement)) throw new Error(`Button not found: ${text}`);
  return button;
}

function setInput(input: HTMLInputElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

function renderSection(overrides: Record<string, unknown> = {}) {
  return render(BotCredentialSection, {
    props: {
      idPrefix: 'test-credential',
      testId: 'credentials',
      items: [item],
      labels,
      createIcon: 'iconify icon-[uil--key-skeleton]',
      oncreate: vi.fn().mockResolvedValue('show-once-secret'),
      onrevoke: vi.fn().mockResolvedValue(true),
      ...overrides
    }
  });
}

describe('BotCredentialSection', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('runs the shared create and revoke lifecycle', async () => {
    const oncreate = vi.fn().mockResolvedValue('show-once-secret');
    const onrevoke = vi.fn().mockResolvedValue(true);
    const { container } = renderSection({ oncreate, onrevoke });

    buttonByText(container, labels.create).click();
    flushSync();
    setInput(container.querySelector('#create-test-credential-name') as HTMLInputElement, 'Backup');
    const createButtons = [...container.querySelectorAll('button')].filter(
      (button) => button.textContent?.trim() === labels.create
    );
    createButtons.at(-1)?.click();

    await vi.waitFor(() => expect(oncreate).toHaveBeenCalledWith('Backup'));
    await vi.waitFor(() => expect(container.textContent).toContain('show-once-secret'));

    buttonByText(container, 'Got it').click();
    flushSync();
    buttonByText(container, labels.revoke).click();
    flushSync();
    const revokeButtons = [...container.querySelectorAll('button')].filter(
      (button) => button.textContent?.trim() === labels.revoke
    );
    revokeButtons.at(-1)?.click();

    await vi.waitFor(() => expect(onrevoke).toHaveBeenCalledWith('credential-1'));
  });

  it('blocks navigation while credential issuance is pending', async () => {
    const pending = deferred<string | null>();
    const { container } = renderSection({ oncreate: () => pending.promise });

    buttonByText(container, labels.create).click();
    flushSync();
    setInput(container.querySelector('#create-test-credential-name') as HTMLInputElement, 'Backup');
    const createButtons = [...container.querySelectorAll('button')].filter(
      (button) => button.textContent?.trim() === labels.create
    );
    createButtons.at(-1)?.click();
    flushSync();

    const cancel = vi.fn();
    const guard = mocks.beforeNavigate.mock.calls.at(-1)?.[0];
    if (!guard) throw new Error('Navigation guard was not registered');
    guard({ cancel });

    expect(cancel).toHaveBeenCalledOnce();
    pending.resolve(null);
    await vi.waitFor(() => expect(createButtons.at(-1)?.disabled).toBe(false));
  });

  it('keeps the create dialog open when creation does not complete', async () => {
    const oncreate = vi.fn().mockResolvedValue(null);
    const { container } = renderSection({ oncreate });

    buttonByText(container, labels.create).click();
    flushSync();
    setInput(container.querySelector('#create-test-credential-name') as HTMLInputElement, 'Backup');
    const createDialog = container.querySelector('dialog[open]');
    buttonByText(createDialog as HTMLDialogElement, labels.create).click();
    await vi.waitFor(() => expect(oncreate).toHaveBeenCalledWith('Backup'));

    expect(createDialog).toHaveAttribute('open');
  });

  it('keeps the revoke dialog open when revocation does not complete', async () => {
    const onrevoke = vi.fn().mockResolvedValue(false);
    const { container } = renderSection({ onrevoke });

    buttonByText(container, labels.revoke).click();
    flushSync();
    const revokeDialog = container.querySelector('dialog[open]');
    buttonByText(revokeDialog as HTMLDialogElement, labels.revoke).click();
    await vi.waitFor(() => expect(onrevoke).toHaveBeenCalledWith('credential-1'));

    expect(revokeDialog).toHaveAttribute('open');
  });

  it('disables creation and explains when the limit is reached', () => {
    const { container } = renderSection({ limit: 1 });

    expect(buttonByText(container, labels.create).disabled).toBe(true);
    expect(container.textContent).toContain(labels.limitReached);
  });
});
