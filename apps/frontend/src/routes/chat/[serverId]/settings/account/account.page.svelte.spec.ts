import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { queryClient } from '$lib/query/client';
import { adminQueryKeys } from '$lib/query/admin';
import AccountPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  listExternalIdentities: vi.fn(),
  listVerifiedEmails: vi.fn(),
  requestEmailVerification: vi.fn(),
  setPrimaryEmail: vi.fn(),
  goto: vi.fn(),
  scopeCurrent: true,
  currentUser: {
    user: {
      id: 'U123abcetc.',
      login: 'alice',
      displayName: 'Alice',
      hasPassword: true,
      viewerCanDeleteAccount: false
    },
    loading: false,
    load: vi.fn()
  }
}));

const connection = {
  queryScope: 'account-settings-test',
  getAPI: () => ({
    list: mocks.listExternalIdentities,
    listVerifiedEmails: mocks.listVerifiedEmails,
    requestEmailVerification: mocks.requestEmailVerification,
    setPrimaryEmail: mocks.setPrimaryEmail
  })
};

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
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

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('Account settings page', () => {
  beforeEach(() => {
    sessionStorage.clear();
    mocks.listExternalIdentities.mockReset();
    mocks.listExternalIdentities.mockResolvedValue({ providers: [], linkedIdentities: [] });
    mocks.listVerifiedEmails.mockReset();
    mocks.listVerifiedEmails.mockResolvedValue([]);
    mocks.requestEmailVerification.mockReset();
    mocks.requestEmailVerification.mockResolvedValue(undefined);
    mocks.setPrimaryEmail.mockReset();
    mocks.goto.mockReset();
    mocks.goto.mockResolvedValue(undefined);
    mocks.scopeCurrent = true;
    queryClient.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows the current user ID in account information', async () => {
    const { container, getByText } = render(AccountPage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(4);
    await expect.element(getByText('User ID')).toBeVisible();
    await expect.element(getByText('U123abcetc.')).toBeVisible();
  });

  it('shows verified email addresses and their primary actions', async () => {
    mocks.listVerifiedEmails.mockResolvedValue([
      { email: 'alice@example.com', primary: true },
      { email: 'alice.secondary@example.com', primary: false }
    ]);

    const { container, getByRole, getByText } = render(AccountPage);
    await settle();

    expect(container.querySelector('table')).not.toBeNull();
    await expect.element(getByText('Email address', { exact: true })).toBeVisible();
    await expect.element(getByRole('row', { name: 'alice@example.com Primary' })).toBeVisible();
    await expect.element(getByRole('button', { name: 'Make primary' })).toBeVisible();

    await getByRole('button', { name: 'Add email address' }).click();

    await expect.element(getByRole('dialog', { name: 'Add email address' })).toBeInTheDocument();
    await expect
      .element(getByRole('textbox', { name: 'Email address', exact: true }))
      .toBeVisible();
    await expect.element(getByRole('textbox', { name: 'Confirm email address' })).toBeVisible();
    await expect.element(getByText(/six-digit verification code/)).toBeVisible();
  });

  it('invalidates admin email views after selecting a primary address', async () => {
    const emails = [
      { email: 'alice@example.com', primary: false },
      { email: 'alice.secondary@example.com', primary: true }
    ];
    mocks.listVerifiedEmails.mockResolvedValue([
      { email: 'alice@example.com', primary: true },
      { email: 'alice.secondary@example.com', primary: false }
    ]);
    mocks.setPrimaryEmail.mockResolvedValue(emails);
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');

    const { getByRole } = render(AccountPage);
    await settle();
    await getByRole('button', { name: 'Make primary' }).click();
    await settle();

    expect(mocks.setPrimaryEmail).toHaveBeenCalledWith('alice.secondary@example.com');
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.membersRoot('origin', connection)
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: adminQueryKeys.member('origin', connection, 'U123abcetc.'),
      exact: true
    });
  });

  it('requires matching email addresses before requesting verification', async () => {
    const { getByRole } = render(AccountPage);
    await settle();

    await getByRole('button', { name: 'Add email address' }).click();

    const emailInput = getByRole('textbox', { name: 'Email address', exact: true });
    const confirmationInput = getByRole('textbox', { name: 'Confirm email address' });
    const submitButton = getByRole('button', { name: 'Send verification code' });

    await emailInput.fill('alice.new@example.com');
    await confirmationInput.fill('alice.typo@example.com');

    await expect.element(confirmationInput).toHaveAttribute('aria-invalid', 'true');
    await expect.element(getByRole('alert')).toHaveTextContent('Email addresses do not match');
    await expect.element(submitButton).toBeDisabled();
    expect(mocks.requestEmailVerification).not.toHaveBeenCalled();

    await confirmationInput.fill('ALICE.NEW@example.com');
    await expect.element(submitButton).toBeEnabled();
    await submitButton.click();

    expect(mocks.requestEmailVerification).toHaveBeenCalledWith('alice.new@example.com');
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/settings/account/verify-email');
  });

  it('does not navigate when the verification request outlives its server scope', async () => {
    let resolveRequest = () => {};
    mocks.requestEmailVerification.mockImplementation(
      () => new Promise<void>((resolve) => (resolveRequest = resolve))
    );

    const { getByRole } = render(AccountPage);
    await settle();
    await getByRole('button', { name: 'Add email address' }).click();
    await getByRole('textbox', { name: 'Email address', exact: true }).fill(
      'alice.new@example.com'
    );
    await getByRole('textbox', { name: 'Confirm email address' }).fill('alice.new@example.com');
    await getByRole('button', { name: 'Send verification code' }).click();

    expect(mocks.requestEmailVerification).toHaveBeenCalledOnce();
    mocks.scopeCurrent = false;
    resolveRequest();
    await settle();

    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
