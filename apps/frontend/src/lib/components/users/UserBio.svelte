<!--
@component

Renders a user's bio with the message Markdown component, including block
formatting, links, and timestamp controls. Source HTML is disabled by the shared
renderer. Plain text remains visible while the component loads or rendering fails.

**Props:**
- `bio` - User-authored Markdown source
- `class` - Optional classes for layout constraints on the owning profile surface
- `timestampSettings` - Viewer preferences for embedded timestamps
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
    bio,
    class: className,
    timestampSettings
  }: {
    bio: string;
    class?: string;
    timestampSettings?: TimeFormatSettings;
  } = $props();
</script>

<div data-testid="user-bio" class={['min-w-0 break-words', className]} dir="auto">
  {#await loadMessageContent()}
    <p class="whitespace-pre-line">{bio}</p>
  {:then { default: MessageContent }}
    <MessageContent body={bio} {timestampSettings} />
  {:catch}
    <p class="whitespace-pre-line">{bio}</p>
  {/await}
</div>
