<!--
@component

Welcomes the owner after first-run setup. When setup marked the onboarding
as pending, the dialog takes the mark once the origin session's viewer is
ready, activates Shield Mode, and suggests the first configuration tasks.
Renders nothing on other servers or later visits.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/client';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { takeOwnerOnboarding } from '$lib/state/ownerOnboarding';
  import { Dialog, Hint } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { toastError } from '$lib/utils/errorMessage';
  import DestinationCard from '../settings/DestinationCard.svelte';
  import { adminNavItem } from './adminNav';

  const scope = useServerScope();
  const store = scope.store;
  const serverId = scope.serverId;
  const serverSegment = serverIdToSegment(serverId);

  let visible = $state(false);
  // Show the inactive hint only after activation fails, not while it runs.
  let shieldFailed = $state(false);
  const privilegedMode = $derived(store.projection.viewer?.privilegedMode);

  // Take the setup hand-off once the viewer, and with it the privileged-mode
  // availability, has loaded. Later runs find no pending mark.
  $effect(() => {
    if (!privilegedMode || !serverRegistry.isOriginServer(serverId)) return;
    if (!takeOwnerOnboarding()) return;
    visible = true;
    if (privilegedMode.available && !privilegedMode.active) {
      store.setPrivilegedMode(true).catch((error: unknown) => {
        shieldFailed = true;
        toastError(error, m('chat.owner_welcome.shield_failed'));
      });
    }
  });

  const nextSteps = (['general', 'rooms', 'registration', 'invites'] as const).map((destination) =>
    adminNavItem(destination, serverSegment)
  );
</script>

<Dialog bind:visible title={m('chat.owner_welcome.title')} size="lg">
  <div class="flex flex-col gap-6" data-testid="owner-welcome">
    <p>{m('chat.owner_welcome.intro')}</p>
    {#if privilegedMode?.active}
      <Hint tone="success" icon="icon-[uil--shield-check]">
        {m('chat.owner_welcome.shield_active')}
      </Hint>
    {:else if shieldFailed}
      <Hint tone="warning" icon="icon-[uil--shield]">
        {m('chat.owner_welcome.shield_inactive')}
      </Hint>
    {/if}
    <section class="flex flex-col gap-3">
      <h3 class="font-bold text-text-top">{m('chat.owner_welcome.next_steps')}</h3>
      <nav
        aria-label={m('chat.owner_welcome.next_steps')}
        class="grid grid-cols-1 gap-4 sm:grid-cols-2"
      >
        {#each nextSteps as item (item.href)}
          <DestinationCard {...item} onclick={() => (visible = false)} />
        {/each}
      </nav>
    </section>
  </div>
  {#snippet primaryAction()}
    <Button defaultAction onclick={() => (visible = false)}>
      {m('chat.owner_welcome.start')}
    </Button>
  {/snippet}
</Dialog>
