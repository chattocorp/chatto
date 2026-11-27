<!--
@component

The welcome message of the current server, rendered as Markdown. The viewer
can collapse it to its title band and expand it again; the choice is kept per
server on this device. Renders nothing when there is no welcome message.
-->
<script lang="ts">
  import MarkdownContent from '$lib/components/MarkdownContent.svelte';
  import { m } from '$lib/i18n/messages';
  import { isWelcomeCollapsed, setWelcomeCollapsed } from '$lib/storage/overviewDismissals';
  import { Panel } from '$lib/ui';

  let { serverId, welcomeMessage = null }: { serverId: string; welcomeMessage?: string | null } =
    $props();

  /** The viewer's choice in this view; until then, the stored choice applies. */
  let collapsedNow = $state<boolean | null>(null);
  const collapsed = $derived(collapsedNow ?? isWelcomeCollapsed(serverId));

  function toggle() {
    const next = !collapsed;
    setWelcomeCollapsed(serverId, next);
    collapsedNow = next;
  }
</script>

{#if welcomeMessage?.trim()}
  <div data-testid="server-overview-welcome">
    <Panel title={m('chat.overview.welcome_title')} {collapsed}>
      {#snippet titleContent()}
        <!-- The title is the disclosure control, as in the sidebar's room groups. -->
        <button
          type="button"
          class="-mx-1 flex cursor-pointer items-center gap-2 rounded-md px-1 focus-visible:outline-2 focus-visible:outline-action"
          aria-expanded={!collapsed}
          onclick={toggle}
        >
          <span
            aria-hidden="true"
            class={[
              'iconify icon-[uil--angle-right-b] text-muted transition-transform',
              collapsed ? 'rtl:-scale-x-100' : 'rotate-90'
            ]}
          ></span>
          {m('chat.overview.welcome_title')}
        </button>
      {/snippet}
      <MarkdownContent source={welcomeMessage} />
    </Panel>
  </div>
{/if}
