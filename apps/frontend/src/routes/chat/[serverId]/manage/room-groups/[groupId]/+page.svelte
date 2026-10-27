<!-- @component General room-group settings with protection for unsaved changes. -->
<script lang="ts">
  import { beforeNavigate, goto } from '$app/navigation';
  import type { BeforeNavigate } from '@sveltejs/kit';
  import { base, resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { ConfirmDialog } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useRoomGroupManagement } from './roomGroupManagementContext';
  import RoomGroupGeneralSettingsPanel from './RoomGroupGeneralSettingsPanel.svelte';
  const serverScope = useServerScope();
  const detail = useRoomGroupManagement();
  /** Each section exposes its current draft state directly to the navigation guard. */
  type SettingsSection = { hasChanges(): boolean };
  let generalSection = $state<SettingsSection>();
  let pendingNavigation = $state<BeforeNavigate | null>(null);
  // Approval applies only to the resumed destination, not to later navigation attempts.
  let approvedDestination: string | null = null;
  let approvedUnload = false;

  beforeNavigate((navigation) => {
    const { cancel, type, to } = navigation;
    if (type === 'leave' && approvedUnload) {
      approvedUnload = false;
      return;
    }
    if (to && approvedDestination === to.url.href) {
      approvedDestination = null;
      return;
    }
    // Permission loss must still be able to remove this editor and its private drafts.
    if (!detail.canManageGroup) {
      pendingNavigation = null;
      return;
    }
    const dirty = generalSection?.hasChanges();
    if (!dirty) return;
    cancel();
    // Closing or reloading a tab must use the browser's native unload warning.
    if (type !== 'leave' && to) pendingNavigation ??= navigation;
  });

  /** Resumes the selected destination without prompting twice or adding a back-history entry. */
  async function discardAndLeave(): Promise<void> {
    const navigation = pendingNavigation;
    pendingNavigation = null;
    if (!navigation?.to) return;
    if (navigation.type === 'popstate' && navigation.delta) {
      approvedDestination = navigation.to.url.href;
      window.history.go(navigation.delta);
    } else if (navigation.willUnload) {
      approvedUnload = true;
      window.location.assign(navigation.to.url.href);
    } else {
      approvedDestination = navigation.to.url.href;
      try {
        const { pathname, search, hash } = navigation.to.url;
        // The captured URL already contains the deployment base. Resolve the app-relative path.
        const path = pathname.slice(base.length) || '/';
        await goto(resolve(`${path}${search}${hash}` as '/'));
      } catch {
        // Another navigation guard can still cancel. Keep this page's drafts intact.
      } finally {
        approvedDestination = null;
      }
    }
  }

  // Permission changes must send viewers to a section they can use.
  $effect(() => {
    if (detail.canManageGroup) return;
    void goto(
      resolve('/chat/[serverId]/manage/room-groups/[groupId]/permissions', {
        serverId: serverIdToSegment(serverScope.serverId),
        groupId: detail.groupId
      }),
      { replaceState: true }
    );
  });
</script>

{#if detail.canManageGroup}
  {#if pendingNavigation}
    <ConfirmDialog
      title={m('admin.rooms_admin.settings.discard_changes')}
      tone="warning"
      actionLabel={m('admin.rooms_admin.settings.discard_changes')}
      onconfirm={discardAndLeave}
      onclose={() => (pendingNavigation = null)}
    >
      {m('admin.rooms_admin.settings.unsaved_changes')}
    </ConfirmDialog>
  {/if}
  {#key `${detail.group.id}:${detail.formRevision}`}
    <RoomGroupGeneralSettingsPanel
      bind:this={generalSection}
      group={detail.group}
      saving={detail.saving}
      onSave={detail.saveGeneralSettings}
    />
  {/key}
{/if}
