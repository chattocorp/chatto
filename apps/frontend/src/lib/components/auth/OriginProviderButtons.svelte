<!-- @component Provider sign-in choices for the origin login and registration pages. -->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import {
    openProviderSignIn,
    verifyProviderSignIn,
    type ProviderSignInResult
  } from '$lib/auth/providerSignIn';
  import type { OAuthPopup } from '$lib/oauth/popup';
  import AuthProviderButtons from './AuthProviderButtons.svelte';
  import AuthPopupNotice from './AuthPopupNotice.svelte';

  let {
    providers,
    href,
    popupUrl,
    compact = false,
    selectedProviderId = $bindable(null),
    onstart,
    onresult,
    onerror
  }: {
    providers: readonly PublicAuthProvider[];
    href: (provider: PublicAuthProvider) => string;
    /** Omit for continuations that must navigate in the current window. */
    popupUrl?: (provider: PublicAuthProvider) => string;
    compact?: boolean;
    selectedProviderId?: string | null;
    onstart: () => void;
    onresult: (result: ProviderSignInResult) => Promise<void>;
    onerror: (error: unknown) => void;
  } = $props();

  let pending = $state.raw<OAuthPopup | null>(null);
  let attempt = 0;
  onDestroy(() => {
    attempt++;
    pending?.cancel();
  });

  async function choose(event: MouseEvent, provider: PublicAuthProvider) {
    event.preventDefault();
    const current = ++attempt;
    pending?.cancel();
    pending = null;
    onstart();
    selectedProviderId = provider.id;
    if (!popupUrl) {
      window.location.href = href(provider);
      return;
    }
    try {
      pending = openProviderSignIn(popupUrl(provider));
      const result = await verifyProviderSignIn(pending);
      if (current !== attempt) return;
      pending = null;
      await onresult(result);
    } catch (error) {
      if (current === attempt) onerror(error);
    } finally {
      if (current === attempt) {
        pending = null;
        selectedProviderId = null;
      }
    }
  }
</script>

<AuthProviderButtons
  {providers}
  {href}
  {compact}
  {selectedProviderId}
  retrySelected={pending !== null}
  onclick={choose}
/>
<AuthPopupNotice pending={pending !== null} oncancel={() => pending?.cancel()} />
