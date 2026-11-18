import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { oauthPopupChannelName, OAUTH_POPUP_RESPONSE_TYPE } from '$lib/oauth/popup';
import OriginProviderButtons from './OriginProviderButtons.svelte';

const windows = vi.hoisted(() => ({ open: vi.fn() }));
vi.mock('$lib/oauth/authorizationWindow', () => ({
  openAuthorizationWindow: windows.open,
  authorizationWindowFeatures: () => 'popup'
}));

it('retries in one click and accepts only the replacement callback', async () => {
  const urls: string[] = [];
  const close = vi.fn();
  windows.open.mockImplementation(() => ({
    navigate: async (url: string) => urls.push(url),
    detachOpener: vi.fn(),
    isClosed: async () => true,
    close
  }));
  const provider = {
    id: 'handle',
    type: 'atproto',
    label: 'Internet Handle',
    loginUrl: '/auth/providers/handle',
    autoProvision: true,
    issuerUrl: null
  };
  const onresult = vi.fn();
  const onerror = vi.fn();
  const screen = render(OriginProviderButtons, {
    props: {
      providers: [provider],
      href: () => provider.loginUrl,
      popupUrl: () => provider.loginUrl,
      onstart: vi.fn(),
      onresult,
      onerror
    }
  });
  await screen.getByRole('link', { name: 'Continue with Internet Handle' }).click();
  await screen.getByRole('button', { name: 'Try Again' }).click();
  expect(close).toHaveBeenCalledOnce();
  expect(urls).toHaveLength(2);
  for (const [index, url] of urls.entries()) {
    const redirect = new URL(url).searchParams.get('redirect')!;
    const state = new URL(redirect, location.origin).searchParams.get('state')!;
    const channel = new BroadcastChannel(oauthPopupChannelName(state));
    channel.postMessage({
      type: OAUTH_POPUP_RESPONSE_TYPE,
      state,
      externalIdentityToken: String(index)
    });
    channel.close();
  }
  await vi.waitFor(() =>
    expect(onresult).toHaveBeenCalledExactlyOnceWith({ kind: 'confirm-account', token: '1' })
  );
  expect(onerror).not.toHaveBeenCalled();
  await expect
    .element(screen.getByRole('link', { name: 'Continue with Internet Handle' }))
    .toBeVisible();
});
