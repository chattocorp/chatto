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
    vi.clearAllMocks();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('prevents navigation while credential issuance is pending', () => {
    render(ShowOnceCredentialDialog, { ...requiredProps, pending: true });

    expect(attemptNavigation()).toHaveBeenCalledOnce();
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
