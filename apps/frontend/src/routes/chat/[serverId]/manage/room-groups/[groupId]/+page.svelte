<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { createAdminRoomLayoutAPI, type AdminRoomGroup } from '$lib/api-client/adminRoomLayout';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { Panel } from '$lib/components/admin';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';

  const groupId = $derived(page.params.groupId!);
  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const connection = useConnection();
  const backHref = $derived(resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment }));

  let group = $state<AdminRoomGroup | null>(null);
  let loading = $state(true);
  let saving = $state(false);
  let name = $state('');
  let description = $state('');
  let originalName = $state('');
  let originalDescription = $state('');
  let loadId = 0;
  const changed = $derived(
    name.trim() !== originalName || description.trim() !== originalDescription
  );

  function applyGroup(nextGroup: AdminRoomGroup): void {
    group = nextGroup;
    name = nextGroup.name;
    description = nextGroup.description ?? '';
    originalName = nextGroup.name;
    originalDescription = nextGroup.description ?? '';
  }

  async function loadGroup(targetGroupId: string) {
    const thisId = ++loadId;
    loading = true;
    group = null;
    try {
      const conn = connection();
      const api = createAdminRoomLayoutAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const groups = await api.listRoomGroups();
      if (thisId !== loadId) return;
      const nextGroup = groups.find((candidate) => candidate.id === targetGroupId) ?? null;
      if (nextGroup) applyGroup(nextGroup);
    } catch {
      if (thisId === loadId) group = null;
    } finally {
      if (thisId === loadId) loading = false;
    }
  }

  $effect(() => {
    void loadGroup(groupId);
  });

  async function saveGeneralSettings(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (saving || !name.trim() || !changed) return;

    saving = true;
    try {
      const conn = connection();
      const api = createAdminRoomLayoutAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const updated = await api.updateRoomGroup({
        groupId,
        name: name.trim(),
        description: description.trim() || null
      });
      if (!updated) throw new Error('Room group update returned no group');

      applyGroup(updated);
      void serverRegistry.getStore(activeServerId).adminRoomLayout.refresh();
      toast.success(m['admin.rooms_admin.group_renamed']());
    } catch (error) {
      toast.error(
        m['admin.rooms_admin.rename_group_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    } finally {
      saving = false;
    }
  }

  const pageTitle = $derived(
    group
      ? `${group.name} · ${m['admin.rooms_admin.rename_group']()}`
      : m['admin.rooms_admin.rename_group']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

{#if loading}
  <!-- The management shell remains visible while the room group loads. -->
{:else if group}
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader
      title={group.name}
      subtitle={m['admin.rooms_admin.rename_group']()}
      {backHref}
      backLabel={m['admin.rooms_admin.back_to_rooms']()}
      showMobileNav
    />

    <div class="flex flex-col gap-6 overflow-y-auto p-6">
      <Panel title={m['admin.nav.general']()} icon="iconify uil--setting">
        <form class="flex max-w-2xl flex-col gap-4" onsubmit={saveGeneralSettings}>
          <TextInput
            id="room-group-settings-name"
            label={m['admin.rooms_admin.group_name']()}
            bind:value={name}
            required
            maxlength={80}
            disabled={saving}
          />
          <TextArea
            id="room-group-settings-description"
            label={m['rbac.role_form.description']()}
            bind:value={description}
            rows={3}
            maxlength={500}
            disabled={saving}
          />
          <div class="flex justify-end">
            <Button type="submit" loading={saving} disabled={!name.trim() || !changed}>
              {m['admin.permissions.save_changes']()}
            </Button>
          </div>
        </form>
      </Panel>

      <div class="flex flex-col gap-4">
        <h2 class="text-lg font-semibold text-text-top">
          {m['admin.rooms_admin.group_permissions_title_fallback']()}
        </h2>
        <Hint>{m['admin.rooms_admin.group_permissions_hint']()}</Hint>
        <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
        <PermissionMatrix {groupId} />
      </div>
    </div>
  </div>
{/if}
