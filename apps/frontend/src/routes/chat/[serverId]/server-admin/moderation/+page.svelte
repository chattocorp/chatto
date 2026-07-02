<script lang="ts">
  import { onMount } from 'svelte';
  import { createRoomCommandAPI, type RoomBanSummary } from '@chatto/api-client/rooms';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UnbanRoomMemberModal from '$lib/components/moderation/UnbanRoomMemberModal.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { toast } from '$lib/ui/toast';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import * as m from '$lib/i18n/messages';

  const userSettings = getUserSettings();
  const activeLocale = $derived(getLocale());
  const connection = useConnection();

  let bans = $state.raw<RoomBanSummary[]>([]);
  let unbanningBanId = $state<string | null>(null);
  let unbanDialogBan = $state<RoomBanSummary | null>(null);
  let unbanError = $state<string | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let loadRequest = 0;

  function roomAPI() {
    const conn = connection();
    return createRoomCommandAPI({
      serverId: conn.serverId ?? getActiveServer(),
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  }

  async function loadRoomBans() {
    const request = ++loadRequest;
    loading = true;
    error = null;
    try {
      const api = roomAPI();
      const nextBans: RoomBanSummary[] = [];
      let offset = 0;
      let hasMore = true;
      while (hasMore) {
        const page = await api.listRoomBans({ limit: 100, offset });
        nextBans.push(...page.bans);
        hasMore = page.hasMore;
        offset += page.bans.length;
        if (page.bans.length === 0) break;
      }
      if (request !== loadRequest) return;
      bans = nextBans;
    } catch (err) {
      if (request !== loadRequest) return;
      error = m['admin.moderation.admin_unavailable']();
      console.error('Failed to load room bans:', err);
    } finally {
      if (request === loadRequest) {
        loading = false;
      }
    }
  }

  onMount(() => {
    void loadRoomBans();
  });

  function formatDate(value: string | null | undefined): string {
    if (!value) return m['admin.moderation.no_expiry']();
    return formatDateUtil(value, userSettings, activeLocale);
  }

  function roomLabel(ban: RoomBanSummary): string {
    return ban.room ? `#${ban.room.name}` : ban.roomId;
  }

  function openUnbanDialog(ban: RoomBanSummary) {
    unbanDialogBan = ban;
    unbanError = null;
  }

  async function unban(ban: RoomBanSummary, reason: string) {
    if (unbanningBanId) return;
    unbanningBanId = ban.id;
    unbanError = null;
    try {
      await roomAPI().unbanRoomMember({
        roomId: ban.roomId,
        userId: ban.userId,
        reason
      });
    } catch (error) {
      unbanningBanId = null;
      unbanError = m['admin.moderation.unban_failed']();
      toast.error(unbanError);
      console.error('Failed to unban room member:', error);
      return;
    }
    unbanningBanId = null;

    toast.success(m['admin.moderation.unban_success']());
    unbanDialogBan = null;
    await loadRoomBans();
  }
</script>

<PageTitle title={m['admin.common.page_title']({ title: m['admin.moderation.title']() })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.moderation.title']()}
    subtitle={m['admin.moderation.subtitle']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">{m['admin.moderation.loading_bans']()}</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      <Panel noPadding>
        <DataTable items={bans} columns={5} emptyMessage={m['admin.moderation.empty_bans']()}>
          {#snippet header()}
            <th class="px-3 py-2 font-medium">{m['admin.common.user']()}</th>
            <th class="px-3 py-2 font-medium">{m['admin.common.room']()}</th>
            <th class="px-3 py-2 font-medium">{m['admin.common.reason']()}</th>
            <th class="px-3 py-2 font-medium">{m['admin.common.expires']()}</th>
            <th class="px-3 py-2 font-medium"></th>
          {/snippet}
          {#snippet row(ban)}
            {@const user = ban.user}
            <td class="min-w-48 px-3 py-2">
              <div class="flex items-center gap-2">
                {#if user}
                  <UserAvatar {user} size="sm" />
                {:else}
                  <div
                    class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-200 text-muted"
                  >
                    <span class="iconify text-base uil--user"></span>
                  </div>
                {/if}
                <div class="min-w-0">
                  <div class="truncate font-medium">{user?.displayName || ban.userId}</div>
                  <div class="truncate text-xs text-muted">
                    {#if user}@{user.login}{/if}
                  </div>
                </div>
              </div>
            </td>
            <td class="max-w-56 px-3 py-2">
              <div class="truncate">{roomLabel(ban)}</div>
            </td>
            <td class="min-w-64 px-3 py-2">
              <div class="line-clamp-2 break-words whitespace-pre-wrap">{ban.reason}</div>
            </td>
            <td class="px-3 py-2 text-muted">
              <div class="whitespace-nowrap">{formatDate(ban.expiresAt)}</div>
            </td>
            <td class="px-3 py-2 text-right">
              <Button
                variant="secondary"
                size="sm"
                loading={unbanningBanId === ban.id}
                loadingText={m['admin.moderation.unbanning']()}
                onclick={() => openUnbanDialog(ban)}
              >
                <span class="iconify uil--unlock"></span>
                <span>{m['admin.moderation.unban']()}</span>
              </Button>
            </td>
          {/snippet}
        </DataTable>
      </Panel>
    {/if}
  </div>
</div>

{#if unbanDialogBan}
  <UnbanRoomMemberModal
    user={unbanDialogBan.user}
    userId={unbanDialogBan.userId}
    room={unbanDialogBan.room}
    roomId={unbanDialogBan.roomId}
    submitting={unbanningBanId === unbanDialogBan.id}
    error={unbanError}
    onconfirm={(reason) => unban(unbanDialogBan!, reason)}
    onclose={() => (unbanDialogBan = null)}
  />
{/if}
