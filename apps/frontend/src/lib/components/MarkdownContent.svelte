<!--
@component

User- or server-authored Markdown, such as a bio, a welcome message, or the
message of the day. It renders with the message renderer, so Chatto room,
server, and message links become pills. Source HTML is disabled by the shared
renderer. The plain text shows while the renderer loads or if it fails.
-->
<script module lang="ts">
  let contentModule: Promise<typeof import('$lib/components/MessageContent.svelte')> | null = null;

  function loadMessageContent() {
    contentModule ??= import('$lib/components/MessageContent.svelte');
    return contentModule;
  }
</script>

<script lang="ts">
  import type { TimeFormatSettings } from '$lib/utils/formatTime';

  let {
    source,
    timestampSettings
  }: {
    source: string;
    /** Viewer preferences for embedded timestamps. */
    timestampSettings?: TimeFormatSettings;
  } = $props();
</script>

{#await loadMessageContent()}
  <p class="whitespace-pre-line">{source}</p>
{:then { default: MessageContent }}
  <MessageContent body={source} {timestampSettings} />
{:catch}
  <p class="whitespace-pre-line">{source}</p>
{/await}
