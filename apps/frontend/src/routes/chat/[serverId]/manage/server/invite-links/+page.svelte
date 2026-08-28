<script lang="ts">
  import { createInfiniteQuery, createMutation } from '@tanstack/svelte-query';
  import { createInviteLinkAPI, type InviteLink } from '$lib/api-client/invitations';
  import Panel from '$lib/ui/Panel.svelte';
  import DataTable from '$lib/ui/DataTable.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { ConfirmDialog, Hint, PaneContent, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, Checkbox, Select, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

  const PAGE_SIZE = 20;
  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  let scrollContainer = $state<HTMLDivElement>();
  let maxUses = $state('1');
  let unlimitedUses = $state(false);
  let expiry = $state('7d');
  let revokeTarget = $state<InviteLink | null>(null);

  const invitationsQuery = createInfiniteQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: adminQueryKeys.invitations(serverId, connection),
        queryFn: ({ pageParam, signal }) =>
          connection.getAPI(createInviteLinkAPI).list(pageParam, PAGE_SIZE, { signal }),
        initialPageParam: 0,
        getNextPageParam: (lastPage, _pages, lastPageParam) =>
          lastPage.hasMore && lastPage.inviteLinks.length > 0
            ? lastPageParam + lastPage.inviteLinks.length
            : undefined
      };
    },
    () => queryClient
  );

  const invitations = $derived(
    (invitationsQuery.data?.pages ?? []).flatMap((page) => page.inviteLinks)
  );
  const loading = $derived(invitationsQuery.isPending);
  const loadingMore = $derived(invitationsQuery.isFetchingNextPage);
  const hasMore = $derived(invitationsQuery.hasNextPage);
  const maxUsesValue = $derived(unlimitedUses ? null : Number.parseInt(maxUses, 10));
  const maxUsesInvalid = $derived(
    !unlimitedUses && (!Number.isInteger(maxUsesValue) || (maxUsesValue ?? 0) < 1)
  );

  const createInvitationMutation = createMutation(
    () => ({
      mutationFn: () =>
        serverScope.connection.getAPI(createInviteLinkAPI).create({
          maxUses: maxUsesValue,
          expiresAt: expiryDate(expiry)
        }),
      onSuccess: async () => {
        await queryClient.invalidateQueries({
          queryKey: adminQueryKeys.invitations(serverScope.serverId, serverScope.connection)
        });
        toast.success(m('admin.invitations.created'));
      },
      onError: showError
    }),
    () => queryClient
  );

  const revokeMutation = createMutation(
    () => ({
      mutationFn: (invitation: InviteLink) =>
        serverScope.connection.getAPI(createInviteLinkAPI).revoke(invitation.id),
      onSuccess: async () => {
        revokeTarget = null;
        await queryClient.invalidateQueries({
          queryKey: adminQueryKeys.invitations(serverScope.serverId, serverScope.connection)
        });
        toast.success(m('admin.invitations.revoked'));
      },
      onError: showError
    }),
    () => queryClient
  );

  function expiryDate(value: string): string | null {
    const days = value === '1d' ? 1 : value === '30d' ? 30 : value === 'never' ? 0 : 7;
    return days === 0 ? null : new Date(Date.now() + days * 86_400_000).toISOString();
  }

  async function copyInvitation(invitation: InviteLink) {
    await navigator.clipboard.writeText(invitation.link);
    toast.success(m('admin.invitations.copied'));
  }

  function formatTimestamp(value: string): string {
    return formatDateTime(value, userSettings, activeLocale);
  }

  function showError(error: unknown) {
    toast.error(error instanceof Error ? error.message : String(error));
  }

  async function loadMore() {
    if (!loading && !loadingMore && hasMore) await invitationsQuery.fetchNextPage();
  }

  function statusTone(status: InviteLink['status']): 'success' | 'danger' | 'muted' {
    return status === 'active' ? 'success' : status === 'revoked' ? 'danger' : 'muted';
  }
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.invitations.title') })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.invitations.title')}
    subtitle={m('admin.invitations.subtitle')}
    showMobileNav
  />

  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      <Panel title={m('admin.invitations.create_title')} icon="iconify icon-[uil--envelope-share]">
        <form
          class="flex flex-col gap-4"
          onsubmit={(event) => {
            event.preventDefault();
            if (!maxUsesInvalid) createInvitationMutation.mutate();
          }}
        >
          <div class="grid gap-4 sm:grid-cols-2">
            <TextInput
              id="invitation-max-uses"
              label={m('admin.invitations.max_uses')}
              bind:value={maxUses}
              disabled={unlimitedUses || createInvitationMutation.isPending}
              error={maxUsesInvalid ? m('admin.invitations.max_uses_error') : undefined}
            />
            <Select
              id="invitation-expiry"
              label={m('admin.invitations.expiry')}
              bind:value={expiry}
              disabled={createInvitationMutation.isPending}
              options={[
                { value: '1d', label: m('admin.invitations.expiry_1d') },
                { value: '7d', label: m('admin.invitations.expiry_7d') },
                { value: '30d', label: m('admin.invitations.expiry_30d') },
                { value: 'never', label: m('admin.invitations.expiry_never') }
              ]}
            />
          </div>
          <Checkbox
            id="invitation-unlimited-uses"
            bind:checked={unlimitedUses}
            label={m('admin.invitations.unlimited_uses')}
            disabled={createInvitationMutation.isPending}
          />
          <div>
            <Button
              type="submit"
              loading={createInvitationMutation.isPending}
              disabled={maxUsesInvalid}
            >
              <span class="iconify icon-[uil--plus]"></span>
              {m('admin.invitations.create')}
            </Button>
          </div>
        </form>
      </Panel>

      <Panel title={m('admin.invitations.list_title')} noPadding>
        {#if invitationsQuery.error}
          <div class="p-5"><Hint tone="danger">{String(invitationsQuery.error)}</Hint></div>
        {/if}
        <DataTable
          items={invitations}
          columns={5}
          emptyMessage={m('admin.invitations.empty')}
          {hasMore}
          {loadingMore}
          onLoadMore={loadMore}
          loadMoreRoot={scrollContainer}
          loadingMoreMessage={m('admin.common.loading')}
        >
          {#snippet header()}
            <th class="table-header-cell">{m('admin.common.status')}</th>
            <th class="table-header-cell">{m('admin.invitations.uses')}</th>
            <th class="table-header-cell">{m('admin.common.expires')}</th>
            <th class="table-header-cell">{m('admin.invitations.created_at')}</th>
            <th class="table-header-cell text-end">{m('admin.invitations.actions')}</th>
          {/snippet}
          {#snippet row(invitation)}
            <td class="px-4 py-3"
              ><Pill tone={statusTone(invitation.status)}
                >{m(`admin.invitations.status.${invitation.status}`)}</Pill
              ></td
            >
            <td class="px-4 py-3 whitespace-nowrap"
              >{invitation.useCount} / {invitation.maxUses ?? '∞'}</td
            >
            <td class="px-4 py-3 text-muted"
              >{invitation.expiresAt
                ? formatTimestamp(invitation.expiresAt)
                : m('admin.invitations.never')}</td
            >
            <td class="px-4 py-3 text-muted">{formatTimestamp(invitation.createdAt)}</td>
            <td class="px-4 py-3">
              <div class="flex justify-end gap-2">
                <Button size="sm" variant="secondary" onclick={() => copyInvitation(invitation)}>
                  <span class="iconify icon-[uil--copy]"></span>{m('admin.invitations.copy')}
                </Button>
                {#if invitation.status === 'active'}
                  <Button size="sm" variant="danger" onclick={() => (revokeTarget = invitation)}>
                    <span class="iconify icon-[uil--ban]"></span>{m('admin.invitations.revoke')}
                  </Button>
                {/if}
              </div>
            </td>
          {/snippet}
        </DataTable>
        {#if loading && invitations.length === 0}
          <div class="p-5 text-muted">{m('admin.common.loading')}</div>
        {/if}
      </Panel>
    </div>
  </PaneContent>
</div>

{#if revokeTarget}
  <ConfirmDialog
    title={m('admin.invitations.revoke_title')}
    actionLabel={m('admin.invitations.revoke')}
    loading={revokeMutation.isPending}
    onconfirm={() => revokeTarget && revokeMutation.mutate(revokeTarget)}
    onclose={() => (revokeTarget = null)}
  >
    {m('admin.invitations.revoke_description')}
  </ConfirmDialog>
{/if}
