import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import {
  readPendingEmailVerification,
  storePendingEmailVerification
} from '$lib/verifiedEmailChallenge';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';
import { settingsQueryKeys } from '$lib/query/settings';
import VerifyEmailPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  confirmEmailVerification: vi.fn(),
  requestEmailVerification: vi.fn(),
  goto: vi.fn(),
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
    mocks.toastSuccess.mockReset();
    mocks.scopeCurrent = true;
    queryClient.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('ignores a confirmation response after its server scope is disposed', async () => {
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
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
    expect(invalidateQueries).not.toHaveBeenCalled();
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(readPendingEmailVerification('origin', 'U123abcetc.')).toBe(
      'alice.new@example.com'
    );
  });

  it('invalidates admin email views after confirming an address', async () => {
    const emails = [{ email: 'alice.new@example.com', primary: true }];
    mocks.confirmEmailVerification.mockResolvedValue(emails);
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');

    const { getByRole } = render(VerifyEmailPage);
    await getByRole('textbox', { name: 'Digit 1' }).fill('123456');
    await settle();
    await getByRole('button', { name: 'Verify email' }).click();
    await settle();

    expect(mocks.confirmEmailVerification).toHaveBeenCalledWith(
      'alice.new@example.com',
      '123456'
    );
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalled());
    expect(queryClient.getQueryData(settingsQueryKeys.verifiedEmails('origin', connection))).toEqual(
      emails
    );
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.membersRoot('origin', connection)
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.member('origin', connection, 'U123abcetc.'),
      exact: true
    });
  });
});
