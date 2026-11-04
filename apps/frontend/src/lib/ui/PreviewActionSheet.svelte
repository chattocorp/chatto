<!-- @component
Renders the preview's shared action definitions as labelled sheet entries.
PreviewFrame owns the gesture and open state; actions close this sheet before
calling their owner.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import ContextMenu from './ContextMenu.svelte';
  import MenuSection from './MenuSection.svelte';
  import { providePreviewActionSheet } from './previewActionSheetContext';

  let { children, onclose }: { children: Snippet; onclose: () => void } = $props();
  providePreviewActionSheet(() => onclose());
</script>

<ContextMenu presentation="sheet" role="group" ariaLabel={m('room.message.actions.more')} {onclose}>
  <MenuSection>{@render children()}</MenuSection>
</ContextMenu>
