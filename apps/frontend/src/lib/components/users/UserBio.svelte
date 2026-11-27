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
<script lang="ts">
  import MarkdownContent from '$lib/components/MarkdownContent.svelte';
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
  <MarkdownContent source={bio} {timestampSettings} />
</div>
