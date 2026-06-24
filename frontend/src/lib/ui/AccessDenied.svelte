<script lang="ts">
  import * as m from '$lib/i18n/messages';

  let {
    message = m['ui.access_denied.message'](),
    backHref = '/chat',
    backLabel = m['ui.access_denied.back']()
  }: {
    message?: string;
    backHref?: string;
    backLabel?: string;
  } = $props();
</script>

<div class="flex h-full w-full flex-col items-center justify-center gap-4">
  <div class="text-6xl text-danger">{m['ui.access_denied.title']()}</div>
  <div class="text-lg text-muted">{message}</div>
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- backHref is a prop; callers pass already-resolved paths -->
  <a href={backHref} class="link">{backLabel}</a>
</div>
