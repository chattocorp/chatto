<!--
@component
Asks the user to sign in again through a linked provider before a protected
account action continues (fresh-auth re-verification, FDR-023). Explains
what happens next, shows an optional error, and offers one button for each
provider that can re-verify. Nothing leaves the page until the user clicks.
-->
<script lang="ts">
  import type { ExternalIdentityProviderInfo } from '$lib/api/externalIdentities';
  import { providerIcon } from '$lib/auth/providerIcon';
  import { m } from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let {
    providers,
    body,
    error = '',
    busyProviderId = '',
    size = 'md',
    onselect
  }: {
    /** Providers that can re-verify the user. */
    providers: readonly ExternalIdentityProviderInfo[];
    /** Explains the action that continues after the re-verification. */
    body: string;
    /** Error from an earlier attempt, or empty. */
    error?: string;
    /** Provider whose re-verification is starting, or empty. Disables every button. */
    busyProviderId?: string;
    size?: 'md' | 'lg';
    onselect: (provider: ExternalIdentityProviderInfo) => void;
  } = $props();
</script>

<div class="flex flex-col gap-4">
  {#if error}
    <Hint tone="danger">{error}</Hint>
  {/if}
  <p class="text-muted">{body}</p>
  {#each providers as provider (provider.id)}
    <Button
      variant="secondary"
      {size}
      fullWidth
      loading={busyProviderId === provider.id}
      disabled={busyProviderId !== ''}
      onclick={() => onselect(provider)}
    >
      <span aria-hidden="true" class={['iconify', providerIcon(provider.type)]}></span>
      {m('settings.account.sso.reverify_modal.action', { provider: provider.label })}
    </Button>
  {/each}
</div>
