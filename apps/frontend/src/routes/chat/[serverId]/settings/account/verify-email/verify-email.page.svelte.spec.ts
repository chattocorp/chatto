import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import {
  readPendingEmailVerification,
  storePendingEmailVerification
} from '$lib/verifiedEmailChallenge';
import VerifyEmailPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  confirmEmailVerification: vi.fn(),
  requestEmailVerification: vi.fn(),
  goto: vi.fn(),
  setQueryData: vi.fn(),
  toastSuccess: vi.fn(),
  scopeCurrent: true
}));

const connection = {
  queryScope: 'verify-email-test',
  getAPI: () => ({
    confirmEmailVerification: mocks.confirmEmailVerification,
    requestEmailVerification: mocks.requestEmailVerification
  })
};

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: mocks.goto
}));

vi.mock('$lib/query/client', () => ({
  queryClient: { setQueryData: mocks.setQueryData }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: { currentUser: { user: { id: 'U123abcetc.' } } },
    connection,
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('$lib/ui/toast/toastState.svelte', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/ui/toast/toastState.svelte')>()),
  toast: { success: mocks.toastSuccess }
}));

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('Verify email page', () => {
  beforeEach(() => {
    sessionStorage.clear();
    storePendingEmailVerification('origin', 'U123abcetc.', 'alice.new@example.com');
    mocks.confirmEmailVerification.mockReset();
    mocks.requestEmailVerification.mockReset();
    mocks.goto.mockReset();
    mocks.goto.mockResolvedValue(undefined);
    mocks.setQueryData.mockReset();
    mocks.toastSuccess.mockReset();
    mocks.scopeCurrent = true;
  });

  it('ignores a confirmation response after its server scope is disposed', async () => {
    let resolveConfirmation = (_emails: Array<{ email: string; primary: boolean }>) => {};
    mocks.confirmEmailVerification.mockImplementation(
      () =>
        new Promise<Array<{ email: string; primary: boolean }>>(
          (resolve) => (resolveConfirmation = resolve)
        )
    );

    const { getByRole } = render(VerifyEmailPage);
    await getByRole('textbox', { name: 'Digit 1' }).fill('123456');
    await getByRole('button', { name: 'Verify email' }).click();

    expect(mocks.confirmEmailVerification).toHaveBeenCalledWith(
      'alice.new@example.com',
      '123456'
    );
    mocks.scopeCurrent = false;
    resolveConfirmation([{ email: 'alice.new@example.com', primary: true }]);
    await settle();

    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.setQueryData).not.toHaveBeenCalled();
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(readPendingEmailVerification('origin', 'U123abcetc.')).toBe(
      'alice.new@example.com'
    );
  });
});
