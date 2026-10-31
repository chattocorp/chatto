<!-- @component Shared provider choices. The caller owns navigation and authentication. -->
<script lang="ts">
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import { Button } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { providerIcon } from '$lib/auth/providerIcon';

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
    href?: (provider: PublicAuthProvider) => string;
    onclick: (event: MouseEvent, provider: PublicAuthProvider) => void;
  } = $props();
</script>

<div class="flex flex-col gap-3">
  {#each providers as provider (provider.id)}
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
  {/each}
</div>
