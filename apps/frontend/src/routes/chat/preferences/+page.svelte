<script lang="ts">
  import SettingsOverview from '$lib/components/settings/SettingsOverview.svelte';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';

  const groups = $derived(getSettingsNavGroups());
</script>

<SettingsOverview {groups} />
