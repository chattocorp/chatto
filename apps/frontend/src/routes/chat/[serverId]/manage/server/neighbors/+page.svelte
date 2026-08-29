<script lang="ts">
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { createNeighborAPI, type Neighbor } from '$lib/api-client/neighbors';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { m } from '$lib/i18n/messages';
  import { ConfirmDialog, Hint, PaneContent } from '$lib/ui';
  import DataTable from '$lib/ui/DataTable.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const serverScope = useServerScope();
  let newOrigin = $state('');
  let editOrigin = $state('');

  type NeighborMutationVariables = {
    serverId: string;
    connection: ServerConnection;
    queryKey: ReturnType<typeof adminQueryKeys.neighbors>;
  };

  type CreateVariables = NeighborMutationVariables & { origin: string };
  type UpdateVariables = NeighborMutationVariables & { neighbor: Neighbor; origin: string };
  type DeleteVariables = NeighborMutationVariables & { neighbor: Neighbor };

  let editTarget = $state<UpdateVariables | null>(null);
  let deleteTarget = $state<DeleteVariables | null>(null);

  const neighborsQuery = createQuery(
    () => ({
      queryKey: adminQueryKeys.neighbors(serverScope.serverId, serverScope.connection),
      queryFn: ({ signal }) => serverScope.connection.getAPI(createNeighborAPI).list({ signal })
    }),
    () => queryClient
  );

  const createMutationState = createMutation(
    () => ({
      mutationFn: ({ connection, origin }: CreateVariables) =>
        connection.getAPI(createNeighborAPI).create(origin),
      onSuccess: (neighbor, variables) => {
        if (!isCurrent(variables)) return;
        queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) => [
          ...current,
          neighbor
        ]);
        newOrigin = '';
        toast.success(m('admin.neighbors.created'));
      },
      onError: (error, variables) => {
        if (isCurrent(variables)) showError(error);
      }
    }),
    () => queryClient
  );

  const updateMutationState = createMutation(
    () => ({
      mutationFn: ({ connection, neighbor, origin }: UpdateVariables) =>
        connection.getAPI(createNeighborAPI).update(neighbor, origin),
      onSuccess: (updated, variables) => {
        if (!isCurrent(variables)) return;
        queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) =>
          current.map((neighbor) => (neighbor.id === updated.id ? updated : neighbor))
        );
        editTarget = null;
        editOrigin = '';
        toast.success(m('admin.neighbors.updated'));
      },
      onError: (error, variables) => {
        if (isCurrent(variables)) showError(error);
      }
    }),
    () => queryClient
  );

  const deleteMutationState = createMutation(
    () => ({
      mutationFn: ({ connection, neighbor }: DeleteVariables) =>
        connection.getAPI(createNeighborAPI).delete(neighbor),
      onSuccess: (_result, variables) => {
        if (!isCurrent(variables)) return;
        queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) =>
          current.filter((neighbor) => neighbor.id !== variables.neighbor.id)
        );
        deleteTarget = null;
        if (editTarget?.neighbor.id === variables.neighbor.id) editTarget = null;
        toast.success(m('admin.neighbors.deleted'));
      },
      onError: (error, variables) => {
        if (isCurrent(variables)) showError(error);
      }
    }),
    () => queryClient
  );

  const neighbors = $derived(neighborsQuery.data ?? []);

  function startEdit(neighbor: Neighbor) {
    editTarget = { ...mutationVariables(), neighbor, origin: neighbor.origin };
    editOrigin = neighbor.origin;
  }

  function mutationVariables(): NeighborMutationVariables {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      serverId,
      connection,
      queryKey: adminQueryKeys.neighbors(serverId, connection)
    };
  }

  function isCurrent(variables: NeighborMutationVariables): boolean {
    return (
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope
    );
  }

  function cancelEdit() {
    editTarget = null;
    editOrigin = '';
  }

  function showError(error: unknown) {
    toast.error(error instanceof Error ? error.message : String(error));
  }
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.neighbors.title') })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.neighbors.title')}
    subtitle={m('admin.neighbors.subtitle')}
    showMobileNav
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      <Panel title={m('admin.neighbors.add_title')} icon="iconify icon-[uil--server-connection]">
        <form
          class="flex max-w-3xl items-end gap-3"
          onsubmit={(event) => {
            event.preventDefault();
            if (newOrigin.trim())
              createMutationState.mutate({ ...mutationVariables(), origin: newOrigin });
          }}
        >
          <div class="min-w-0 flex-1">
            <TextInput
              id="new-neighbor-origin"
              label={m('admin.neighbors.origin')}
              placeholder="https://chat.example"
              bind:value={newOrigin}
              disabled={createMutationState.isPending}
            />
          </div>
          <Button
            type="submit"
            loading={createMutationState.isPending}
            disabled={!newOrigin.trim()}
          >
            <span class="iconify icon-[uil--plus]"></span>
            {m('admin.neighbors.add')}
          </Button>
        </form>
        <p class="mt-3 text-sm text-muted">{m('admin.neighbors.origin_help')}</p>
      </Panel>

      <Panel title={m('admin.neighbors.list_title')} noPadding>
        {#if neighborsQuery.error}
          <div class="p-5"><Hint tone="danger">{String(neighborsQuery.error)}</Hint></div>
        {/if}
        <DataTable items={neighbors} columns={2} emptyMessage={m('admin.neighbors.empty')}>
          {#snippet header()}
            <th class="table-header-cell">{m('admin.neighbors.origin')}</th>
            <th class="table-header-cell text-end">{m('admin.neighbors.actions')}</th>
          {/snippet}
          {#snippet row(neighbor)}
            <td class="px-4 py-3">
              {#if editTarget?.neighbor.id === neighbor.id}
                <TextInput
                  id={`neighbor-origin-${neighbor.id}`}
                  label={m('admin.neighbors.origin')}
                  labelHidden
                  bind:value={editOrigin}
                  disabled={updateMutationState.isPending}
                />
              {:else}
                <span class="font-mono text-sm break-all">{neighbor.origin}</span>
              {/if}
            </td>
            <td class="px-4 py-3">
              <div class="flex justify-end gap-2">
                {#if editTarget?.neighbor.id === neighbor.id}
                  <Button size="sm" variant="secondary" onclick={cancelEdit}>
                    {m('admin.neighbors.cancel')}
                  </Button>
                  <Button
                    size="sm"
                    loading={updateMutationState.isPending}
                    disabled={!editOrigin.trim() || editOrigin === neighbor.origin}
                    onclick={() =>
                      editTarget &&
                      updateMutationState.mutate({ ...editTarget, origin: editOrigin })}
                  >
                    {m('admin.neighbors.save')}
                  </Button>
                {:else}
                  <Button size="sm" variant="secondary" onclick={() => startEdit(neighbor)}>
                    <span class="iconify icon-[uil--edit]"></span>
                    {m('admin.neighbors.edit')}
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    onclick={() => (deleteTarget = { ...mutationVariables(), neighbor })}
                  >
                    <span class="iconify icon-[uil--trash-alt]"></span>
                    {m('admin.neighbors.delete')}
                  </Button>
                {/if}
              </div>
            </td>
          {/snippet}
        </DataTable>
        {#if neighborsQuery.isPending && neighbors.length === 0}
          <div class="p-5 text-muted">{m('admin.common.loading')}</div>
        {/if}
      </Panel>
    </div>
  </PaneContent>
</div>

{#if deleteTarget}
  <ConfirmDialog
    title={m('admin.neighbors.delete_title')}
    actionLabel={m('admin.neighbors.delete')}
    loading={deleteMutationState.isPending}
    onconfirm={() => deleteTarget && deleteMutationState.mutate(deleteTarget)}
    onclose={() => (deleteTarget = null)}
  >
    {m('admin.neighbors.delete_description', { origin: deleteTarget.neighbor.origin })}
  </ConfirmDialog>
{/if}
