<!--
@component

The welcome message of the current server, rendered as Markdown. The viewer
can hide it on this device. Renders nothing when there is no welcome message.
-->
<script lang="ts">
  import MarkdownContent from '$lib/components/MarkdownContent.svelte';
  import { m } from '$lib/i18n/messages';
  import { dismissOverviewCard, isOverviewCardDismissed } from '$lib/storage/overviewDismissals';
  import { CompactActionButton, Panel } from '$lib/ui';

  let { serverId, welcomeMessage = null }: { serverId: string; welcomeMessage?: string | null } =
    $props();

  let dismissedNow = $state(false);
  const visible = $derived(
    !!welcomeMessage?.trim() && !dismissedNow && !isOverviewCardDismissed(serverId, 'welcome')
  );

  function dismiss() {
    dismissOverviewCard(serverId, 'welcome');
    dismissedNow = true;
  }
</script>

{#if visible && welcomeMessage}
  <div data-testid="server-overview-welcome">
    <Panel title={m('chat.overview.welcome_title')}>
      {#snippet actions()}
        <CompactActionButton label={m('chat.overview.hide_welcome')} onclick={dismiss}>
          <span aria-hidden="true" class="iconify icon-[uil--times]"></span>
        </CompactActionButton>
      {/snippet}
      <MarkdownContent source={welcomeMessage} />
    </Panel>
  </div>
{/if}
