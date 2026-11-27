import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { captureMutationCompletion } from './mutationCompletion';
import { getAdminNavItems } from '$lib/components/chat/adminNav';
import { serverIdToSegment } from '$lib/navigation';
import { useServerScope } from '$lib/state/server/scope.svelte';
import { useShieldMode } from '$lib/state/server/shieldMode';
import { toast } from '$lib/ui/toast';
import { m } from '$lib/i18n/messages';

/** Build the sidebar and Overview destinations from this session's current and preview grants. */
export function useSettingsManagementNav() {
  const scope = useServerScope();
  const shield = useShieldMode();
  const serverSegment = serverIdToSegment(scope.serverId);
  const store = scope.store;
  function items(includePreview: boolean) {
    const preview = includePreview ? store.privilegedPermissionPreview : undefined;
    return getAdminNavItems({
      serverSegment,
      permissions: store.permissions,
      limitedFeatures: store.serverInfo.limitedFeatures,
      preview: preview && { ...preview, requestAccess }
    });
  }

  function requestAccess(href: string) {
    const isCurrent = captureMutationCompletion(scope);
    shield.open(() => {
      if (!isCurrent()) return;
      if (items(false).some((item) => item.href === href)) {
        void goto(resolve(href as '/'));
      } else {
        toast.error(m('common.error.permission_denied'));
      }
    });
  }

  return () => items(true);
}
