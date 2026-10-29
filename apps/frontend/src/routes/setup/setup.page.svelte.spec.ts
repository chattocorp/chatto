import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import SetupPage from './+page.svelte';

const mocks = vi.hoisted(() => ({ complete: vi.fn(), discovery: vi.fn(), goto: vi.fn() }));
// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$lib/api/setup', () => ({ completeServerSetup: mocks.complete }));
vi.mock('@chatto/client/api/server', () => ({ getPublicServerInfo: mocks.discovery }));
vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: mocks.goto
}));

function renderSetup(step = 'details', directLoginEnabled = true) {
  const view = render(SetupPage, {
    props: {
      data: {
        step,
        user: null,
        serverInfo: null,
        serverInfoLoaded: true,
        setupServer: {
          name: 'Chatto',
          version: '0.5.0',
          authorizeUrl: '',
          directRegistrationEnabled: false,
          directLoginEnabled,
          accountCreationPolicy: 'open',
          welcomeMessage: null,
          description: null,
          iconUrl: null,
          bannerUrl: null,
          authProviders: [],
          setupRequired: true
        }
      }
    }
  });
  return view;
}

describe('first-run wizard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.discovery.mockResolvedValue({ setupRequired: true });
  });

  it('welcomes the installer with a navigable link to the form', async () => {
    const view = renderSetup('welcome');
    await expect.element(view.getByRole('heading', { name: 'Chat just got real!' })).toBeVisible();
    expect(view.container.querySelectorAll('form')).toHaveLength(0);
    expect(mocks.complete).not.toHaveBeenCalled();
    await expect
      .element(view.getByRole('link', { name: 'Set up your server' }))
      .toHaveAttribute('href', '/setup?step=details');
  });

  it('shows the form on its own URL with a link back to the welcome screen', async () => {
    const view = renderSetup();
    await expect.element(view.getByLabelText('Server name')).toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'Back', exact: true }))
      .toHaveAttribute('href', '/setup');
    await expect.element(view.getByLabelText('Username', { exact: true })).toBeVisible();
    expect(view.container.querySelectorAll('form')).toHaveLength(1);
    expect(view.container.getAnimations({ subtree: true })).toHaveLength(0);
  });

  it('shows configuration guidance immediately when password login is disabled', async () => {
    const view = renderSetup('welcome', false);
    await expect
      .element(
        view.getByText(
          'Enable direct login in the server configuration to create a local owner account.'
        )
      )
      .toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'Set up your server' }))
      .not.toBeInTheDocument();
    expect(view.container.querySelectorAll('form')).toHaveLength(0);
  });

  it('keeps the draft after validation failure and submits all fields together', async () => {
    mocks.complete
      .mockRejectedValueOnce(
        new ConnectError('This username is unavailable', Code.InvalidArgument, {
          'Chatto-Error-Field': 'login'
        })
      )
      .mockResolvedValueOnce(undefined);
    const view = await renderSetup();
    await view.getByLabelText('Server name').fill('Our community');
    await view.getByLabelText('Description (optional)').fill('Our description');
    await view.getByLabelText('Username', { exact: true }).fill('founder');
    await view.getByLabelText('Display name').fill('Founder');
    await view.getByLabelText('Password', { exact: true }).fill('password123');
    await view.getByLabelText('Confirm Password', { exact: true }).fill('password123');
    await view.getByRole('button', { name: 'Finish setup' }).click();
    await expect.element(view.getByRole('alert')).toHaveTextContent('This username is unavailable');
    await expect
      .element(view.getByLabelText('Username', { exact: true }))
      .toHaveAttribute('aria-describedby', 'setup-login-error');
    await expect.element(view.getByLabelText('Username', { exact: true })).toHaveValue('founder');
    await expect.element(view.getByLabelText('Server name')).toHaveValue('Our community');
    await view.getByRole('button', { name: 'Finish setup' }).click();
    expect(mocks.complete).toHaveBeenLastCalledWith(window.location.origin, {
      serverName: 'Our community',
      description: 'Our description',
      login: 'founder',
      displayName: 'Founder',
      password: 'password123'
    });
    expect(mocks.goto).toHaveBeenCalledWith('/login', { invalidateAll: true });
  });

  it.each(['a'.repeat(73), 'a'.repeat(128), '😀'.repeat(18) + 'a', '😀'.repeat(32)])(
    'rejects an overlength password and accepts a multibyte correction',
    async (password) => {
      mocks.complete.mockResolvedValue(undefined);
      const view = await renderSetup();
      await view.getByLabelText('Server name').fill('Community');
      await view.getByLabelText('Username', { exact: true }).fill('founder');
      await view.getByLabelText('Display name').fill('Founder');
      await view.getByLabelText('Password', { exact: true }).fill(password);
      await view.getByLabelText('Confirm Password', { exact: true }).fill(password);
      await view.getByRole('button', { name: 'Finish setup' }).click();
      await expect.element(view.getByRole('alert')).toHaveTextContent('Password is too long');
      expect(mocks.complete).not.toHaveBeenCalled();
      await view.getByLabelText('Password', { exact: true }).fill('😀😀');
      await view.getByLabelText('Confirm Password', { exact: true }).fill('😀😀');
      await view.getByRole('button', { name: 'Finish setup' }).click();
      expect(mocks.complete).toHaveBeenCalledWith(
        window.location.origin,
        expect.objectContaining({ password: '😀😀' })
      );
    }
  );

  it('updates mismatch feedback as either password changes without submitting', async () => {
    const view = renderSetup();
    const password = view.getByLabelText('Password', { exact: true });
    const confirmation = view.getByLabelText('Confirm Password', { exact: true });
    await password.fill('password123');
    await expect.element(view.getByRole('alert')).not.toBeInTheDocument();
    await confirmation.fill('different123');
    await expect.element(view.getByRole('alert')).toHaveTextContent('Passwords do not match');
    await expect.element(confirmation).toHaveAttribute('aria-invalid', 'true');
    await view.getByLabelText('Username', { exact: true }).fill('founder');
    await expect.element(view.getByRole('alert')).toHaveTextContent('Passwords do not match');
    await confirmation.fill('password123');
    await expect.element(view.getByRole('alert')).not.toBeInTheDocument();
    await expect.element(confirmation).not.toHaveAttribute('aria-invalid');
    await password.fill('changed123');
    await expect.element(view.getByRole('alert')).toHaveTextContent('Passwords do not match');
    await password.fill('password123');
    await expect.element(view.getByRole('alert')).not.toBeInTheDocument();
    await confirmation.fill('');
    await expect.element(view.getByRole('alert')).not.toBeInTheDocument();
    expect(mocks.complete).not.toHaveBeenCalled();
  });

  it('rejects mismatched confirmation without submitting credentials', async () => {
    const view = await renderSetup();
    await view.getByLabelText('Server name').fill('Community');
    await view.getByLabelText('Username', { exact: true }).fill('founder');
    await view.getByLabelText('Display name').fill('Founder');
    await view.getByLabelText('Password', { exact: true }).fill('password123');
    await view.getByLabelText('Confirm Password', { exact: true }).fill('different123');
    await view.getByRole('button', { name: 'Finish setup' }).click();
    await expect
      .element(view.getByLabelText('Confirm Password', { exact: true }))
      .toHaveAttribute('aria-describedby', 'setup-password-confirmation-error');
    await expect.element(view.getByRole('alert')).toHaveTextContent('Passwords do not match');
    expect(mocks.complete).not.toHaveBeenCalled();
  });

  it('offers sign-in after a lost response when setup has committed', async () => {
    mocks.complete.mockRejectedValue(new Error('Connection lost'));
    mocks.discovery.mockResolvedValue({ setupRequired: false });
    const view = await renderSetup();
    await view.getByLabelText('Server name').fill('Our community');
    await view.getByLabelText('Username', { exact: true }).fill('founder');
    await view.getByLabelText('Display name').fill('Founder');
    await view.getByLabelText('Password', { exact: true }).fill('password123');
    await view.getByLabelText('Confirm Password', { exact: true }).fill('password123');
    await view.getByRole('button', { name: 'Finish setup' }).click();
    await expect
      .element(view.getByRole('link', { name: 'Sign In' }))
      .toHaveAttribute('href', '/login');
    await expect
      .element(view.getByRole('button', { name: 'Finish setup' }))
      .not.toBeInTheDocument();
  });
});
