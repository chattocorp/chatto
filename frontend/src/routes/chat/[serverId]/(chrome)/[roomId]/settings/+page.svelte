<script lang="ts">
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
</script>

<PageTitle title="Room Settings" />

<PaneHeader title="Dashboard" subtitle="Manage this room" showMobileNav />

<div class="flex flex-1 flex-col items-center justify-center gap-4 text-muted">
  <span class="iconify text-6xl uil--setting"></span>
  <p>Select a section from the sidebar to get started.</p>
</div>
