<script lang="ts">
  import ServerSettings from '$lib/ServerSettings.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
</script>

<PageTitle title="General | Server Admin" />

<PaneHeader title="General" subtitle="Server identity, branding, and messages" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <ServerSettings />
</div>
