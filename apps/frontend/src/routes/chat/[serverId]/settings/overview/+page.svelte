<script lang="ts">
  import SettingsOverview from '$lib/components/settings/SettingsOverview.svelte';
  import { useSettingsManagementNav } from '$lib/navigation/settingsManagementNav';
  import { serverIdToSegment } from '$lib/navigation';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  const scope = useServerScope();
  const managementItems = useSettingsManagementNav();
  const serverSegment = serverIdToSegment(scope.serverId);
  const groups = $derived(
    getSettingsNavGroups({
      serverId: scope.serverId,
      serverSegment,
      managementNavItems: managementItems()
    })
  );
  const privilegedMode = $derived(scope.store.projection.viewer?.privilegedMode);
</script>

<SettingsOverview {groups} showShieldNotice={privilegedMode?.available && !privilegedMode.active} />
