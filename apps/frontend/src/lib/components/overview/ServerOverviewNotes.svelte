<!--
@component

The welcome message and the message of the day of the current server, side by
side on wide screens. The viewer can hide the welcome message on this device.
The message of the day stays: administrators change it, and the app header
shows it too. Renders nothing when neither message is set.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { dismissOverviewCard, isOverviewCardDismissed } from '$lib/storage/overviewDismissals';
  import { CompactActionButton, Panel } from '$lib/ui';
  import ServerMarkdown from '$lib/components/ServerMarkdown.svelte';

  let {
    serverId,
    welcomeMessage = null,
    motd = null
  }: {
    serverId: string;
    welcomeMessage?: string | null;
    motd?: string | null;
  } = $props();

  let dismissedNow = $state(false);
  const showWelcome = $derived(
    !!welcomeMessage?.trim() && !dismissedNow && !isOverviewCardDismissed(serverId, 'welcome')
  );
  const showMotd = $derived(!!motd?.trim());

  function dismissWelcome() {
    dismissOverviewCard(serverId, 'welcome');
    dismissedNow = true;
  }
</script>

{#if showWelcome || showMotd}
  <div
    class={['grid gap-4', showWelcome && showMotd && 'lg:grid-cols-[3fr_2fr]']}
    data-testid="server-overview-notes"
  >
    {#if showWelcome && welcomeMessage}
      <Panel title={m('chat.overview.welcome_title')}>
        {#snippet actions()}
          <CompactActionButton label={m('chat.overview.hide_welcome')} onclick={dismissWelcome}>
            <span aria-hidden="true" class="iconify icon-[uil--times]"></span>
          </CompactActionButton>
        {/snippet}
        <ServerMarkdown source={welcomeMessage} />
      </Panel>
    {/if}
    {#if showMotd && motd}
      <Panel title={m('common.message_of_the_day')} icon="iconify icon-[uil--megaphone]">
        <ServerMarkdown source={motd} />
      </Panel>
    {/if}
  </div>
{/if}
