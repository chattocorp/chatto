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
  beforeNavigate: vi.fn(),
  goto: vi.fn(),
  toastSuccess: vi.fn(),
  scopeCurrent: true,
  currentUser: { user: { id: 'U123abcetc.' } }
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
  beforeNavigate: mocks.beforeNavigate,
  goto: mocks.goto
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: { currentUser: mocks.currentUser },
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
    mocks.beforeNavigate.mockReset();
    mocks.goto.mockReset();
    mocks.goto.mockResolvedValue(undefined);
    mocks.toastSuccess.mockReset();
    mocks.scopeCurrent = true;
    mocks.currentUser.user = { id: 'U123abcetc.' };
    queryClient.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('clears a consumed challenge without stale UI effects after its server scope is disposed', async () => {
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
      'U123abcetc.',
      'alice.new@example.com',
      '123456'
    );
    mocks.scopeCurrent = false;
    resolveConfirmation([{ email: 'alice.new@example.com', primary: true }]);
    await settle();

    expect(mocks.goto).not.toHaveBeenCalled();
    expect(invalidateQueries).not.toHaveBeenCalled();
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(readPendingEmailVerification('origin', 'U123abcetc.')).toBe('');
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
      'U123abcetc.',
      'alice.new@example.com',
      '123456'
    );
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalled());
    expect(
      queryClient.getQueryData(
        settingsQueryKeys.verifiedEmails('origin', connection, 'U123abcetc.')
      )
    ).toEqual(emails);
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.membersRoot('origin', connection)
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.member('origin', connection, 'U123abcetc.'),
      exact: true
    });
  });

  it('reconciles a confirmation without navigating after another navigation starts', async () => {
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    let resolveConfirmation = (_emails: Array<{ email: string; primary: boolean }>) => {};
    mocks.confirmEmailVerification.mockImplementation(
      () =>
        new Promise<Array<{ email: string; primary: boolean }>>(
          (resolve) => (resolveConfirmation = resolve)
        )
    );

    const { getByRole, getByText } = render(VerifyEmailPage);
    await getByRole('textbox', { name: 'Digit 1' }).fill('123456');
    await getByRole('button', { name: 'Verify email' }).click();

    expect(mocks.confirmEmailVerification).toHaveBeenCalledOnce();
    const navigationGuard = mocks.beforeNavigate.mock.calls.at(-1)?.[0] as (() => void) | undefined;
    expect(navigationGuard).toBeTypeOf('function');
    navigationGuard?.();
    resolveConfirmation([{ email: 'alice.new@example.com', primary: true }]);
    await settle();

    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(
      queryClient.getQueryData(
        settingsQueryKeys.verifiedEmails('origin', connection, 'U123abcetc.')
      )
    ).toEqual([{ email: 'alice.new@example.com', primary: true }]);
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.membersRoot('origin', connection)
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.member('origin', connection, 'U123abcetc.'),
      exact: true
    });
    expect(readPendingEmailVerification('origin', 'U123abcetc.')).toBe('');
    await expect.element(getByText('No email verification is in progress.')).toBeVisible();
  });

  it('keeps a retryable challenge when confirmation fails after navigation starts', async () => {
    let rejectConfirmation = (_reason: Error) => {};
    mocks.confirmEmailVerification.mockImplementation(
      () => new Promise((_resolve, reject) => (rejectConfirmation = reject))
    );

    const { getByRole } = render(VerifyEmailPage);
    await getByRole('textbox', { name: 'Digit 1' }).fill('123456');
    await getByRole('button', { name: 'Verify email' }).click();

    const navigationGuard = mocks.beforeNavigate.mock.calls.at(-1)?.[0] as (() => void) | undefined;
    expect(navigationGuard).toBeTypeOf('function');
    navigationGuard?.();
    rejectConfirmation(new Error('verification failed'));
    await settle();

    expect(readPendingEmailVerification('origin', 'U123abcetc.')).toBe('alice.new@example.com');
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
  });
});
