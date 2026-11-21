import '../../app.css';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { PublicServerInfo } from '@chatto/client/api/server';
import AuthLayout from './AuthLayout.svelte';

const children = createRawSnippet(() => ({ render: () => '<form>Sign-in form</form>' }));

const serverInfo: Pick<
  PublicServerInfo,
  'name' | 'iconUrl' | 'bannerUrl' | 'description' | 'welcomeMessage'
> = {
  name: 'Example',
  iconUrl: null,
  bannerUrl: null,
  description: 'Directory blurb',
  welcomeMessage: '**Welcome** to Example.'
};

describe('AuthLayout server branding', () => {
  it('shows the welcome message and not the description on sign-in pages', async () => {
    const rendered = render(AuthLayout, { children, title: 'Sign In', serverInfo });
    await expect
      .element(rendered.getByText('Welcome', { exact: true }).first())
      .toBeInTheDocument();
    expect(rendered.container.textContent).not.toContain('Directory blurb');
  });

  it('shows the welcome message in a limited viewport in compact sign-in windows', async () => {
    const rendered = render(AuthLayout, { children, title: 'Sign In', serverInfo, compact: true });
    const welcome = rendered.getByTestId('compact-welcome-message');
    await expect.element(welcome).toBeVisible();
    await expect.element(welcome.getByText('Welcome', { exact: true })).toBeVisible();
    expect(rendered.container.textContent).not.toContain('Directory blurb');
  });

  it('shows no branding area in compact windows without a banner or welcome message', async () => {
    const rendered = render(AuthLayout, {
      children,
      title: 'Sign In',
      serverInfo: { ...serverInfo, welcomeMessage: null },
      compact: true
    });
    await expect.element(rendered.getByText('Sign-in form')).toBeVisible();
    expect(rendered.container.querySelector('[data-page-reveal].mb-5')).toBeNull();
    expect(rendered.container.textContent).not.toContain('Directory blurb');
  });
});
