<script lang="ts">
  import { pushState } from '$app/navigation';
  import { page } from '$app/state';
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { createInvitationCodeAPI, type InvitationCode } from '$lib/api/invitations';
  import { createUserAPI } from '@chatto/client/api/users';
  import UserIdentity from '$lib/components/users/UserIdentity.svelte';
  import { Panel, DataTable, ConfirmDialog, FormDialog, Hint, Pill, CopyButton } from '$lib/ui';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery, createMutation, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { Button, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import {
    formatDate,
    formatDateTime,
    formatMessageTime,
    timeFormatSettingsFor
  } from '$lib/utils/formatTime';

  const PAGE_SIZE = 20;
  // Codes created in this screen always expire seven days after issuance.
  const CODE_LIFETIME_MS = 7 * 24 * 60 * 60 * 1000;
  const NOTE_MAX_LENGTH = 250;
  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  let { scrollContainer }: { scrollContainer?: HTMLDivElement } = $props();
  let revokeTarget = $state<InvitationCode | null>(null);
  let note = $state('');
  const noteLength = $derived([...note].length);
  const creatorId = $props.id();
  const createModal = $derived(page.state.modal);
  const creatingCode = $derived(
    createModal?.type === 'createInvitationCode' &&
      createModal.serverId === serverScope.serverId &&
      createModal.creatorId === creatorId
  );

  function openCreateDialog() {
    if (creatingCode || createInvitationMutation.isPending) return;
    note = '';
    pushState('', {
      modal: { type: 'createInvitationCode', serverId: serverScope.serverId, creatorId }
    });
  }

  function closeCreateDialog() {
    note = '';
    if (creatingCode) history.back();
  }

  function canAccess(invitation: InvitationCode): boolean {
    const permissions = serverScope.store.permissions;
    return (
      permissions.canManageInvites ||
      (permissions.canInviteUsers &&
        invitation.createdBy === serverScope.store.currentUser.user?.id)
    );
  }

  const invitationsQuery = createInfiniteQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      queryKey: [
        ...adminQueryKeys.invitations(serverId, connection),
        { all: serverScope.store.permissions.canManageInvites }
      ],
      queryFn: async ({ pageParam, signal }) => {
        const result = await connection
          .getAPI(createInvitationCodeAPI)
          .list(pageParam, PAGE_SIZE, { signal });
        const creatorIds = [
          ...new Set(result.invitationCodes.map((code) => code.createdBy).filter(Boolean))
        ];
        if (creatorIds.length) await connection.getAPI(createUserAPI).batchGetUsers(creatorIds);
        return result;
      },
      initialPageParam: 0,
      getNextPageParam: (lastPage, _pages, lastPageParam) =>
        lastPage.hasMore && lastPage.invitationCodes.length > 0
          ? lastPageParam + lastPage.invitationCodes.length
          : undefined
    };
  });

  const invitations = $derived(
    (invitationsQuery.data?.pages ?? []).flatMap((page) => page.invitationCodes).filter(canAccess)
  );
  const loading = $derived(invitationsQuery.isPending);
  const loadingMore = $derived(invitationsQuery.isFetchingNextPage);
  const hasMore = $derived(invitationsQuery.hasNextPage);

  const createInvitationMutation = createMutation(() => ({
    mutationFn: (note: string) =>
      serverScope.connection.getAPI(createInvitationCodeAPI).create({
        expiresAt: new Date(Date.now() + CODE_LIFETIME_MS).toISOString(),
        note
      }),
    onSuccess: async () => {
      closeCreateDialog();
      await queryClient.invalidateQueries({
        queryKey: adminQueryKeys.invitations(serverScope.serverId, serverScope.connection)
      });
      toast.success(m('admin.invitations.created'));
    },
    onError: (error) => toastError(error)
  }));

  const revokeMutation = createMutation(() => ({
    mutationFn: (invitation: InvitationCode) =>
      serverScope.connection.getAPI(createInvitationCodeAPI).revoke(invitation.id),
    onSuccess: async () => {
      revokeTarget = null;
      await queryClient.invalidateQueries({
        queryKey: adminQueryKeys.invitations(serverScope.serverId, serverScope.connection)
      });
      toast.success(m('admin.invitations.revoked'));
    },
    onError: (error) => toastError(error)
  }));

  function formatTimestamp(value: string): string {
    return formatDateTime(value, userSettings, activeLocale);
  }

  async function loadMore() {
    if (!loading && !loadingMore && hasMore) await invitationsQuery.fetchNextPage();
  }

  function statusTone(status: InvitationCode['status']): 'success' | 'danger' | 'muted' {
    return status === 'active' ? 'success' : status === 'revoked' ? 'danger' : 'muted';
  }
</script>

<!-- @component Creates, copies, and revokes single-use invitation codes. -->
<Panel
  title={m('admin.invitations.list_title')}
  icon="iconify icon-[uil--envelope-share]"
  noPadding
>
  {#snippet actions()}
    <Button size="sm" disabled={createInvitationMutation.isPending} onclick={openCreateDialog}>
      <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
      {m('admin.invitations.create')}
    </Button>
  {/snippet}
  {#if invitationsQuery.error}
    <div class="p-5"><Hint tone="danger">{errorMessage(invitationsQuery.error)}</Hint></div>
  {/if}
  <DataTable
    items={invitations}
    columns={5}
    getKey={(invitation) => invitation.id}
    loading={loading && invitations.length === 0}
    emptyMessage={m('admin.invitations.empty')}
    {hasMore}
    {loadingMore}
    onLoadMore={loadMore}
    loadMoreRoot={scrollContainer}
    loadingMoreMessage={m('admin.common.loading')}
  >
    {#snippet header()}
      <th class="table-header-cell">{m('auth.register.invitation.label')}</th>
      <th class="table-header-cell">{m('admin.common.status')}</th>
      <th class="table-header-cell">{m('admin.invitations.created_by')}</th>
      <th class="table-header-cell">{m('admin.common.expires')}</th>
      <th class="table-header-cell text-end">{m('admin.invitations.actions')}</th>
    {/snippet}
    {#snippet row(invitation)}
      {@const creator = serverScope.store.projection.users.view(invitation.createdBy)}
      <td class="px-4 py-2">
        <div class="flex items-center gap-2">
          <code
            dir="ltr"
            title={`${m('admin.invitations.created_at')} ${formatTimestamp(invitation.createdAt)}`}
            class={[
              'block w-fit font-mono text-base whitespace-nowrap',
              invitation.status !== 'active' && 'text-muted'
            ]}>{invitation.code}</code
          >
          {#if invitation.status === 'active'}
            <CopyButton value={invitation.code} />
          {/if}
        </div>
        {#if invitation.note}
          <p class="mt-1 max-w-md text-base wrap-anywhere text-muted">{invitation.note}</p>
        {/if}
      </td>
      <td class="px-4 py-2 whitespace-nowrap">
        <Pill tone={statusTone(invitation.status)}
          >{m(`admin.invitations.status.${invitation.status}`)}</Pill
        >
      </td>
      <td class="px-4 py-2">
        {#if creator}
          <UserIdentity
            user={creator}
            viewerSettings={serverScope.store.currentUser.user?.settings}
          />
        {:else}
          <bdi class="text-muted">{invitation.createdBy}</bdi>
        {/if}
      </td>
      <td class="px-4 py-2 text-base whitespace-nowrap">
        {#if invitation.expiresAt}
          <time datetime={invitation.expiresAt} title={formatTimestamp(invitation.expiresAt)}>
            <span class="block">{formatDate(invitation.expiresAt, userSettings, activeLocale)}</span
            >
            <span class="block text-sm text-muted"
              >{formatMessageTime(invitation.expiresAt, userSettings, activeLocale)}</span
            >
          </time>
        {:else}
          <span class="text-muted">{m('admin.invitations.never')}</span>
        {/if}
      </td>
      <td class="w-0 px-4 py-2">
        <div class="flex items-center justify-end gap-1">
          {#if invitation.status === 'active'}
            <Button
              size="icon"
              label={m('admin.invitations.revoke')}
              title={m('admin.invitations.revoke')}
              variant="danger-ghost"
              onclick={() => (revokeTarget = invitation)}
            >
              <span aria-hidden="true" class="iconify icon-[uil--ban]"></span>
            </Button>
          {/if}
        </div>
      </td>
    {/snippet}
  </DataTable>
</Panel>

{#if creatingCode}
  <FormDialog
    visible
    size="sm"
    title={m('admin.invitations.create_title')}
    submitLabel={m('admin.invitations.create')}
    submitIcon="iconify icon-[uil--plus]"
    loading={createInvitationMutation.isPending}
    disabled={noteLength > NOTE_MAX_LENGTH}
    onsubmit={() => {
      if (!createInvitationMutation.isPending && noteLength <= NOTE_MAX_LENGTH)
        createInvitationMutation.mutate(note);
    }}
    onclose={closeCreateDialog}
  >
    {#snippet description()}{m('admin.invitations.usage')}{/snippet}
    <TextInput
      id={`${creatorId}-note`}
      label={m('admin.invitations.note_label')}
      placeholder={m('admin.invitations.note_placeholder')}
      description={m('admin.invitations.note_description')}
      error={noteLength > NOTE_MAX_LENGTH ? m('admin.invitations.note_too_long') : undefined}
      trailingText={`${noteLength}/${NOTE_MAX_LENGTH}`}
      disabled={createInvitationMutation.isPending}
      bind:value={note}
    />
  </FormDialog>
{/if}

<!-- A permission-driven close must reach onclose and discard the old target. -->
{#if revokeTarget}
  <ConfirmDialog
    visible={canAccess(revokeTarget)}
    title={m('admin.invitations.revoke_title')}
    actionLabel={m('admin.invitations.revoke')}
    loading={revokeMutation.isPending}
    onconfirm={() => revokeTarget && revokeMutation.mutate(revokeTarget)}
    onclose={() => (revokeTarget = null)}
  >
    {m('admin.invitations.revoke_description')}
  </ConfirmDialog>
{/if}
