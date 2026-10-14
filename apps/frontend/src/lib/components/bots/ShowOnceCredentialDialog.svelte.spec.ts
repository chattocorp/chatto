import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { getToasts, toast } from '$lib/ui/toast';

const mocks = vi.hoisted(() => ({
  beforeNavigate: vi.fn()
}));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  beforeNavigate: mocks.beforeNavigate
}));

import ShowOnceCredentialDialog from './ShowOnceCredentialDialog.svelte';

const requiredProps = {
  title: 'New credential',
  warning: 'Save this credential now.',
  copiedMessage: 'Copied'
};

function attemptNavigation(cancel = vi.fn()) {
  const guard = mocks.beforeNavigate.mock.calls.at(-1)?.[0];
  if (!guard) throw new Error('Navigation guard was not registered');
  guard({ cancel });
  return cancel;
}

describe('ShowOnceCredentialDialog', () => {
  beforeEach(async () => {
    toast.clear();
    vi.clearAllMocks();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  afterEach(() => {
    vi.restoreAllMocks();
    toast.clear();
  });

  it('keeps the credential available after a copy failure and allows retry', async () => {
    const writeText = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockRejectedValueOnce(new Error('private browser detail'))
      .mockResolvedValue(undefined);
    const { container } = render(ShowOnceCredentialDialog, {
      ...requiredProps,
      visible: true,
      value: 'show-once-secret'
    });
    const copy = [...container.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Copy to clipboard'
    )!;

    await userEvent.click(copy);

    expect(container.querySelector('dialog [role="alert"]')?.textContent?.trim()).toBe(
      'Could not copy to clipboard. Select the text and copy it manually.'
    );
    expect(getToasts()).toHaveLength(0);
    expect(container.querySelector('dialog')?.open).toBe(true);
    expect(container.querySelector('code')?.textContent).toBe('show-once-secret');
    expect(attemptNavigation()).toHaveBeenCalledOnce();

    toast.clear();
    await userEvent.click(copy);

    expect(writeText).toHaveBeenNthCalledWith(2, 'show-once-secret');
    expect(getToasts()).toMatchObject([{ tone: 'success', message: 'Copied' }]);
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.querySelector('dialog')?.open).toBe(true);
  });

  it('prevents navigation while credential issuance is pending', () => {
    render(ShowOnceCredentialDialog, { ...requiredProps, pending: true });

    expect(attemptNavigation()).toHaveBeenCalledOnce();
  });

  it('ignores a clipboard failure that arrives after the dialog closes', async () => {
    const pending = Promise.withResolvers<void>();
    vi.spyOn(navigator.clipboard, 'writeText').mockReturnValue(pending.promise);
    const { container, rerender } = render(ShowOnceCredentialDialog, {
      ...requiredProps,
      visible: true,
      value: 'show-once-secret'
    });
    const button = (label: string) =>
      [...container.querySelectorAll('button')].find(
        (element) => element.textContent?.trim() === label
      )!;
    await userEvent.click(button('Copy to clipboard'));
    await userEvent.click(button('Got it'));

    pending.reject(new Error('denied'));
    await pending.promise.catch(() => undefined);
    flushSync();

    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(getToasts()).toHaveLength(0);
    expect(attemptNavigation()).not.toHaveBeenCalled();

    await rerender({ visible: true, value: 'replacement-secret' });
    expect(container.querySelector('dialog')?.open).toBe(true);
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('prevents navigation while a show-once credential is visible', () => {
    render(ShowOnceCredentialDialog, {
      ...requiredProps,
      visible: true,
      value: 'show-once-secret'
    });

    expect(attemptNavigation()).toHaveBeenCalledOnce();
  });

  it('permits navigation after the credential is acknowledged', () => {
    const { container } = render(ShowOnceCredentialDialog, {
      ...requiredProps,
      visible: true,
      value: 'show-once-secret'
    });

    const acknowledge = [...container.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Got it'
    );
    if (!(acknowledge instanceof HTMLButtonElement)) {
      throw new Error('Acknowledgement button was not found');
    }
    acknowledge.click();
    flushSync();

    expect(attemptNavigation()).not.toHaveBeenCalled();
  });
});
