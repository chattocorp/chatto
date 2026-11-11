<!--
@component
Shared provider choices. The caller owns navigation and authentication.
An AT Protocol provider also asks for the handle and suggests the identity
that this device is signed in to (FDR-056).
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import { Button, TextInput } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { providerIcon } from '$lib/auth/providerIcon';
  import {
    atprotoAccount,
    atprotoOAuthClient,
    restoreAtprotoAccount
  } from '$lib/atproto/session.svelte';

  let {
    providers,
    selectedProviderId = null,
    disabled = false,
    compact = false,
    href,
    onclick
  }: {
    providers: readonly PublicAuthProvider[];
    selectedProviderId?: string | null;
    disabled?: boolean;
    compact?: boolean;
    /** Link target of a redirect-based provider. */
    href?: (provider: PublicAuthProvider) => string;
    onclick: (event: Event, provider: PublicAuthProvider, loginHint?: string) => void;
  } = $props();

  const hasAtproto = $derived(providers.some((provider) => provider.type === 'atproto'));
  const knownHandle = $derived(atprotoAccount()?.handle ?? '');
  let typedHandles = $state<Record<string, string>>({});

  onMount(() => {
    if (!hasAtproto) return;
    // Load the OAuth library now, so a PDS sign-in popup opens directly from
    // the click. Restore a stored session for the handle suggestion.
    void atprotoOAuthClient().catch(() => {});
    void restoreAtprotoAccount();
  });

  function handleFor(provider: PublicAuthProvider): string {
    return (typedHandles[provider.id] ?? knownHandle).trim().replace(/^@/, '');
  }
</script>

<div class="flex flex-col gap-3">
  {#each providers as provider (provider.id)}
    {#if provider.type === 'atproto'}
      <form
        class="flex flex-col gap-2"
        onsubmit={(event) => {
          event.preventDefault();
          onclick(event, provider, handleFor(provider) || undefined);
        }}
      >
        <TextInput
          id={`atproto-handle-${provider.id}`}
          label={m('auth.login.atproto_handle_label')}
          labelHidden
          placeholder={m('auth.login.atproto_handle_placeholder')}
          autocomplete="username"
          value={typedHandles[provider.id] ?? knownHandle}
          oninput={(event: Event) =>
            (typedHandles[provider.id] = (event.currentTarget as HTMLInputElement).value)}
          disabled={disabled || selectedProviderId !== null}
        />
        <Button
          type="submit"
          variant="secondary"
          size={compact ? 'md' : 'lg'}
          fullWidth
          disabled={disabled || selectedProviderId !== null}
          loading={selectedProviderId === provider.id}
          loadingText={m('auth.login.connecting_provider', { provider: provider.label })}
        >
          <span aria-hidden="true" class={['iconify text-lg', providerIcon(provider.type)]}></span>
          {m('auth.login.continue_with_provider', { provider: provider.label })}
        </Button>
      </form>
    {:else}
      <Button
        variant="secondary"
        size={compact ? 'md' : 'lg'}
        fullWidth
        href={href?.(provider)}
        disabled={disabled || selectedProviderId !== null}
        loading={selectedProviderId === provider.id}
        loadingText={m('auth.login.connecting_provider', { provider: provider.label })}
        onclick={(event) => onclick(event, provider)}
      >
        <span aria-hidden="true" class={['iconify text-lg', providerIcon(provider.type)]}></span>
        {m('auth.login.continue_with_provider', { provider: provider.label })}
      </Button>
    {/if}
  {/each}
</div>
