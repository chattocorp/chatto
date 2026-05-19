<script lang="ts">
  let {
    message = 'You do not have permission to access this page.',
    backHref = '/chat',
    backLabel = 'Return to Chat'
  }: {
    message?: string;
    backHref?: string;
    backLabel?: string;
  } = $props();
</script>

<div class="flex h-full w-full flex-col items-center justify-center gap-4">
  <div class="text-6xl text-danger">Access Denied</div>
  <div class="text-lg text-muted">{message}</div>
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- backHref is a prop; callers pass already-resolved paths -->
  <a href={backHref} class="link">{backLabel}</a>
</div>
