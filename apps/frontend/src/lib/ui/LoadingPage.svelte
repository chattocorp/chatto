<script lang="ts">
  import * as m from '$lib/i18n/messages';

  let { message = m['ui.loading_page.loading']() }: { message?: string } = $props();
</script>

<div class="flex h-full w-full items-center justify-center">
  <div class="text-lg text-muted">{message}</div>
</div>
