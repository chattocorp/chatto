<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import LoadingFog from './LoadingFog.svelte';

  let { message = m('ui.loading_page.loading') }: { message?: string } = $props();
</script>

<div class="flex h-full w-full items-center justify-center">
  <LoadingFog class="h-full min-h-64 w-full" label={message} />
</div>
