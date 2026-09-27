<!--
@component

Renders a `MessagePreviewCard` and loads its module on first use. The card
reads through TanStack Query, so this keeps the query runtime out of the room
route's initial bundle. After the first load, every card renders at once.

**Props:** the same as `MessagePreviewCard`.
-->
<script module lang="ts">
  import type MessagePreviewCardComponent from './MessagePreviewCard.svelte';

  let loadedCard = $state.raw<typeof MessagePreviewCardComponent | null>(null);
  let loading: Promise<void> | null = null;

  function loadCard(): void {
    loading ??= import('./MessagePreviewCard.svelte').then(
      (module) => {
        loadedCard = module.default;
      },
      () => {
        // Show no preview; the next card that mounts tries again.
        loading = null;
      }
    );
  }
</script>

<script lang="ts">
  import type { ComponentProps } from 'svelte';

  let props: ComponentProps<typeof MessagePreviewCardComponent> = $props();

  loadCard();
</script>

{#if loadedCard}
  {@const Card = loadedCard}
  <Card {...props} />
{/if}
