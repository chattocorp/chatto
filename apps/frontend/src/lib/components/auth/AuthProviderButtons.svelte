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
    showRestrictions = false,
    href,
    onclick
  }: {
    providers: readonly PublicAuthProvider[];
    selectedProviderId?: string | null;
    retrySelected?: boolean;
    disabled?: boolean;
    compact?: boolean;
    /** Explain sign-in-only methods when account creation is available. */
    showRestrictions?: boolean;
    href?: (provider: PublicAuthProvider) => string;
    onclick: (event: MouseEvent, provider: PublicAuthProvider) => void;
  } = $props();
  const groupId = $props.id();
</script>

<div class="flex flex-col gap-3">
  {#each providers as provider (provider.id)}
    {@const retry = retrySelected && selectedProviderId === provider.id}
    {@const restrictionId =
      showRestrictions && provider.autoProvision === false
        ? `${groupId}-restriction-${provider.id}`
        : undefined}
    <div>
      <Button
        describedBy={restrictionId}
        variant="secondary"
        size={compact ? 'md' : 'lg'}
        fullWidth
        href={retry ? undefined : href?.(provider)}
        disabled={disabled || (selectedProviderId !== null && !retry)}
        loading={selectedProviderId === provider.id && !retry}
        onclick={(event) => onclick(event, provider)}
      >
        <span aria-hidden="true" class={['iconify text-lg', providerIcon(provider.type)]}></span>
        {#if retry}{m('common.retry')}{:else}<bdi>{provider.label}</bdi>{/if}
      </Button>
      {#if restrictionId}
        <p id={restrictionId} class="mt-1 text-center text-sm text-muted">
          {m('auth.login.existing_only')}
        </p>
      {/if}
    </div>
  {/each}
</div>
