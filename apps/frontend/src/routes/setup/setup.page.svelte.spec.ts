import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import SetupPage from './+page.svelte';

const mocks = vi.hoisted(() => ({ complete: vi.fn(), discovery: vi.fn(), goto: vi.fn() }));
vi.mock('$lib/api-client/setup', () => ({ completeServerSetup: mocks.complete }));
vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo: mocks.discovery }));
vi.mock('$app/navigation', async (importOriginal) => ({
  ...await importOriginal<typeof import('$app/navigation')>(),
  goto: mocks.goto
}));

function renderSetup() {
  return render(SetupPage, { props: { data: { user: null, serverInfo: null, serverInfoLoaded: true, setupServer: {
      name: 'Chatto', version: '0.5.0', authorizeUrl: '', directRegistrationEnabled: false,
      directLoginEnabled: true, accountCreationPolicy: 'open', welcomeMessage: null,
      description: null, iconUrl: null, bannerUrl: null, authProviders: [], setupRequired: true
    } } } });
}

describe('first-run wizard', () => {
  beforeEach(() => { vi.clearAllMocks(); mocks.discovery.mockResolvedValue({ setupRequired: true }); });

  it('shows the complete form without starting a page-local animation', async () => {
    const view = renderSetup();
    await expect.element(view.getByLabelText('Server name')).toBeVisible();
    await expect.element(view.getByLabelText('Username', { exact: true })).toBeVisible();
    expect(view.container.querySelectorAll('form')).toHaveLength(1);
    expect(view.container.getAnimations({ subtree: true })).toHaveLength(0);
  });

  it('keeps the draft after validation failure and submits all fields together', async () => {
    mocks.complete.mockRejectedValueOnce(new ConnectError('This username is unavailable', Code.InvalidArgument, { 'Chatto-Error-Field': 'login' })).mockResolvedValueOnce(undefined);
    const view = renderSetup();
    await view.getByLabelText('Server name').fill('Our community');
    await view.getByLabelText('Description (optional)').fill('Our description');
    await view.getByLabelText('Username', { exact: true }).fill('founder');
    await view.getByLabelText('Display name').fill('Founder');
    await view.getByLabelText('Password', { exact: true }).fill('password123');
    await view.getByLabelText('Confirm Password', { exact: true }).fill('password123');
    await view.getByRole('button', { name: 'Finish setup' }).click();
    await expect.element(view.getByRole('alert')).toHaveTextContent('This username is unavailable');
    await expect.element(view.getByLabelText('Username', { exact: true })).toHaveAttribute('aria-describedby', 'setup-login-error');
    await expect.element(view.getByLabelText('Username', { exact: true })).toHaveValue('founder');
    await expect.element(view.getByLabelText('Server name')).toHaveValue('Our community');
    await view.getByRole('button', { name: 'Finish setup' }).click();
    expect(mocks.complete).toHaveBeenLastCalledWith(window.location.origin, { serverName:'Our community', description:'Our description', login:'founder', displayName:'Founder', password:'password123' });
    expect(mocks.goto).toHaveBeenCalledWith('/login', { invalidateAll:true });
  });

  it('rejects mismatched confirmation without submitting credentials', async () => {
    const view = renderSetup();
    await view.getByLabelText('Server name').fill('Community');
    await view.getByLabelText('Username', { exact: true }).fill('founder');
    await view.getByLabelText('Display name').fill('Founder');
    await view.getByLabelText('Password', { exact: true }).fill('password123');
    await view.getByLabelText('Confirm Password', { exact: true }).fill('different123');
    await view.getByRole('button', { name: 'Finish setup' }).click();
    await expect.element(view.getByLabelText('Confirm Password', { exact: true })).toHaveAttribute('aria-describedby', 'setup-password-confirmation-error');
    await expect.element(view.getByRole('alert')).toHaveTextContent('Passwords do not match');
    expect(mocks.complete).not.toHaveBeenCalled();
  });

  it('offers sign-in after a lost response when setup has committed', async () => {
    mocks.complete.mockRejectedValue(new Error('Connection lost'));
    mocks.discovery.mockResolvedValue({ setupRequired:false });
    const view = renderSetup();
    await view.getByLabelText('Server name').fill('Our community');
    await view.getByLabelText('Username', { exact:true }).fill('founder');
    await view.getByLabelText('Display name').fill('Founder');
    await view.getByLabelText('Password', { exact:true }).fill('password123');
    await view.getByLabelText('Confirm Password', { exact: true }).fill('password123');
    await view.getByRole('button', { name:'Finish setup' }).click();
    await expect.element(view.getByRole('link', { name:'Sign In' })).toHaveAttribute('href','/login');
    await expect.element(view.getByRole('button', { name:'Finish setup' })).not.toBeInTheDocument();
  });
});
