<!-- @component Shared provider choices. The caller owns navigation and authentication. -->
<script lang="ts">
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import { Button } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { providerIcon } from '$lib/auth/providerIcon';

  let {
    providers,
    selectedProviderId = null,
    retrySelected = false,
    disabled = false,
    compact = false,
    href,
    onclick
  }: {
    providers: readonly PublicAuthProvider[];
    selectedProviderId?: string | null;
    retrySelected?: boolean;
    disabled?: boolean;
    compact?: boolean;
    href?: (provider: PublicAuthProvider) => string;
    onclick: (event: MouseEvent, provider: PublicAuthProvider) => void;
  } = $props();
</script>

<div class="flex flex-col gap-3">
  {#each providers as provider (provider.id)}
    {@const retry = retrySelected && selectedProviderId === provider.id}
    <Button
      variant="secondary"
      size={compact ? 'md' : 'lg'}
      fullWidth
      href={retry ? undefined : href?.(provider)}
      disabled={disabled || (selectedProviderId !== null && !retry)}
      loading={selectedProviderId === provider.id && !retry}
      loadingText={m('auth.login.connecting_provider', { provider: provider.label })}
      onclick={(event) => onclick(event, provider)}
    >
      <span aria-hidden="true" class={['iconify text-lg', providerIcon(provider.type)]}></span>
      {retry
        ? m('common.retry')
        : m('auth.login.continue_with_provider', { provider: provider.label })}
    </Button>
  {/each}
</div>
