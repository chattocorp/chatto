<script lang="ts">
  import SettingsOverview from '$lib/components/settings/SettingsOverview.svelte';
  import { getAdminNavItems } from '$lib/components/chat/adminNav';
  import { serverIdToSegment } from '$lib/navigation';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  const scope = useServerScope();
  const serverSegment = serverIdToSegment(scope.serverId);
  const groups = $derived(
    getSettingsNavGroups({
      serverId: scope.serverId,
      serverSegment,
      managementNavItems: getAdminNavItems({ serverSegment, permissions: scope.store.permissions })
    })
  );
  const privilegedMode = $derived(scope.store.projection.viewer?.privilegedMode);
</script>

<SettingsOverview {groups} showShieldNotice={privilegedMode?.available && !privilegedMode.active} />
