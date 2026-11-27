<!--
@component

The welcome message of the current server, rendered as Markdown. It always
shows; a long message scrolls inside a limited height, so it cannot push the
rooms far down. Renders nothing when there is no welcome message.
-->
<script lang="ts">
  import MarkdownContent from '$lib/components/MarkdownContent.svelte';
  import { m } from '$lib/i18n/messages';
  import { Panel, ScrollFader } from '$lib/ui';

  let { welcomeMessage = null }: { welcomeMessage?: string | null } = $props();
</script>

{#if welcomeMessage?.trim()}
  <div data-testid="server-overview-welcome">
    <Panel title={m('chat.overview.welcome_title')} icon="iconify icon-[uil--smile] size-5">
      <ScrollFader
        top
        bottom
        mask
        maskSize="1rem"
        fill={false}
        scrollClass="max-h-64"
        data-testid="server-overview-welcome-scroll"
      >
        <MarkdownContent source={welcomeMessage} />
      </ScrollFader>
    </Panel>
  </div>
{/if}
