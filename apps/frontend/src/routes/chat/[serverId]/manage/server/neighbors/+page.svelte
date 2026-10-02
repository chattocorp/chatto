<!--
@component

Neighbor administration. Each card shows the public profile from the current
server's cached Neighborhood, so the browser contacts no advertised server.
Discovery picks up a Neighbor change within about fifteen seconds; the page
polls the cache briefly after a change. See FDR-042.
-->
<script lang="ts">
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { onDestroy } from 'svelte';
  import { createNeighborAPI, type Neighbor } from '$lib/api/neighbors';
  import { listNeighborhoodServers, type NeighborhoodServer } from '@chatto/client/api/server';
  import ServerProfileCard from '$lib/components/ServerProfileCard.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createMutation, createQuery, queryClient } from '$lib/query/client';
  import { serverOriginFromInput } from '$lib/serverDirectory';
  import { canonicalServerOrigin } from '@chatto/client/util/serverUrl';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '@chatto/client/server/serverConnection';
  import { m } from '$lib/i18n/messages';
  import {
    ConfirmDialog,
    EmptyState,
    Hint,
    LoadingFog,
    PaneContent,
    PageTitle,
    PaneHeader,
    Panel
  } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const serverScope = useServerScope();
  let newOrigin = $state('');
  let editOrigin = $state('');
  const normalizedNewOrigin = $derived(serverOriginFromInput(newOrigin));
  const normalizedEditOrigin = $derived(serverOriginFromInput(editOrigin));

  type NeighborMutationVariables = {
    serverId: string;
    connection: ServerConnection;
    queryKey: ReturnType<typeof adminQueryKeys.neighbors>;
  };

  type CreateVariables = NeighborMutationVariables & { origin: string };
  type UpdateVariables = NeighborMutationVariables & {
    neighbor: Neighbor;
    origin: string;
  };
  type DeleteVariables = NeighborMutationVariables & { neighbor: Neighbor };

  let editTarget = $state<UpdateVariables | null>(null);
  let deleteTarget = $state<DeleteVariables | null>(null);

  const neighborsQuery = createQuery(() => ({
    queryKey: adminQueryKeys.neighbors(serverScope.serverId, serverScope.connection),
    queryFn: ({ signal }) => serverScope.connection.getAPI(createNeighborAPI).list({ signal })
  }));

  const createMutationState = createMutation(() => ({
    mutationFn: ({ connection, origin }: CreateVariables) =>
      connection.getAPI(createNeighborAPI).create(origin),
    onSuccess: (neighbor, variables) => {
      if (!serverScope.isCurrent()) return;
      queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) => [
        ...current,
        neighbor
      ]);
      newOrigin = '';
      expectDiscovery();
      toast.success(m('admin.neighbors.created'));
    },
    onError: (error) => {
      if (serverScope.isCurrent()) toastError(error);
    }
  }));

  const updateMutationState = createMutation(() => ({
    mutationFn: ({ connection, neighbor, origin }: UpdateVariables) =>
      connection.getAPI(createNeighborAPI).update(neighbor, origin),
    onSuccess: (updated, variables) => {
      if (!serverScope.isCurrent()) return;
      queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) =>
        current.map((neighbor) => (neighbor.id === updated.id ? updated : neighbor))
      );
      editTarget = null;
      editOrigin = '';
      expectDiscovery();
      toast.success(m('admin.neighbors.updated'));
    },
    onError: (error) => {
      if (serverScope.isCurrent()) toastError(error);
    }
  }));

  const deleteMutationState = createMutation(() => ({
    mutationFn: ({ connection, neighbor }: DeleteVariables) =>
      connection.getAPI(createNeighborAPI).delete(neighbor),
    onSuccess: (_result, variables) => {
      if (!serverScope.isCurrent()) return;
      queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) =>
        current.filter((neighbor) => neighbor.id !== variables.neighbor.id)
      );
      deleteTarget = null;
      if (editTarget?.neighbor.id === variables.neighbor.id) editTarget = null;
      toast.success(m('admin.neighbors.deleted'));
    },
    onError: (error) => {
      if (serverScope.isCurrent()) toastError(error);
    }
  }));

  const neighbors = $derived(neighborsQuery.data ?? []);
  /** Origin of the current server. It hosts the cached profile images. */
  const serverOrigin = $derived(new URL(serverScope.connection.connectBaseUrl).origin);

  /** How long the page polls the cached Neighborhood after a Neighbor change. */
  const DISCOVERY_WAIT_MS = 60_000;
  const DISCOVERY_POLL_MS = 3_000;
  /** A Neighbor change is recent, so discovery can still add its profile. */
  let awaitingDiscovery = $state(false);
  let discoveryTimer: ReturnType<typeof setTimeout> | undefined;

  function expectDiscovery() {
    awaitingDiscovery = true;
    clearTimeout(discoveryTimer);
    discoveryTimer = setTimeout(() => (awaitingDiscovery = false), DISCOVERY_WAIT_MS);
  }

  onDestroy(() => clearTimeout(discoveryTimer));

  const neighborhoodQuery = createQuery(() => {
    // Read the flag here so that a Neighbor change updates the poll timer.
    const polling = awaitingDiscovery;
    return {
      queryKey: ['public', 'neighborhood', serverOrigin],
      queryFn: ({ signal }) => listNeighborhoodServers(serverOrigin, { signal }),
      enabled: neighbors.length > 0,
      refetchInterval: (query) =>
        polling &&
        neighbors.some((neighbor) => !cachedProfiles(query.state.data).has(neighbor.origin))
          ? DISCOVERY_POLL_MS
          : false
    };
  });
  const profilesByOrigin = $derived(cachedProfiles(neighborhoodQuery.data));

  /** Map cached Neighborhood profiles by canonical origin. */
  function cachedProfiles(servers: NeighborhoodServer[] = []) {
    return new Map(
      servers.flatMap((server) => {
        const origin = canonicalServerOrigin(server.origin);
        return origin ? [[origin, server.profile] as const] : [];
      })
    );
  }

  /** `undefined` shows a loading card; `null` shows an unavailable profile. */
  function neighborProfile(origin: string) {
    const profile = profilesByOrigin.get(origin);
    if (profile) return profile;
    return neighborhoodQuery.isPending || awaitingDiscovery ? undefined : null;
  }

  function startEdit(neighbor: Neighbor) {
    editTarget = {
      ...mutationVariables(),
      neighbor,
      origin: neighbor.origin
    };
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

  function cancelEdit() {
    editTarget = null;
    editOrigin = '';
  }

  /** Saves the inline origin edit for a Neighbor; Enter in the field submits it. */
  function submitOriginEdit(event: SubmitEvent, neighbor: Neighbor) {
    event.preventDefault();
    if (updateMutationState.isPending) return;
    if (!editTarget || !normalizedEditOrigin || normalizedEditOrigin === neighbor.origin) return;
    updateMutationState.mutate({ ...editTarget, origin: normalizedEditOrigin });
  }
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.neighbors.title') })}
/>

<div class="pane-page">
  <PaneHeader title={m('admin.neighbors.title')} subtitle={m('admin.neighbors.subtitle')} />

  <PaneContent>
    <div class="flex flex-col gap-6">
      <Panel title={m('admin.neighbors.add_title')} icon="iconify icon-[uil--server-connection]">
        <form
          class="flex max-w-3xl flex-col gap-4"
          onsubmit={(event) => {
            event.preventDefault();
            if (normalizedNewOrigin)
              createMutationState.mutate({
                ...mutationVariables(),
                origin: normalizedNewOrigin
              });
          }}
        >
          <div class="min-w-0">
            <TextInput
              id="new-neighbor-origin"
              label={m('admin.neighbors.origin')}
              description={m('admin.neighbors.origin_help')}
              placeholder="chat.example"
              bind:value={newOrigin}
              disabled={createMutationState.isPending}
            />
          </div>
          <div class="flex justify-end">
            <Button
              type="submit"
              loading={createMutationState.isPending}
              disabled={!normalizedNewOrigin}
            >
              <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
              {m('admin.neighbors.add')}
            </Button>
          </div>
        </form>
      </Panel>

      <Panel title={m('admin.neighbors.list_title')} count={neighbors.length || undefined}>
        {#if neighborsQuery.error}
          <div class="mb-4"><Hint tone="danger">{errorMessage(neighborsQuery.error)}</Hint></div>
        {/if}

        {#if neighborsQuery.isPending && neighbors.length === 0}
          <LoadingFog class="h-40 w-full" />
        {:else if neighbors.length === 0}
          <EmptyState icon="icon-[uil--server-connection]" title={m('admin.neighbors.empty')} />
        {:else}
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {#each neighbors as neighbor (neighbor.id)}
              {#snippet actions()}
                {#if editTarget?.neighbor.id === neighbor.id}
                  <form
                    class="flex flex-col gap-3"
                    onsubmit={(event) => submitOriginEdit(event, neighbor)}
                  >
                    <TextInput
                      id={`neighbor-origin-${neighbor.id}`}
                      label={m('admin.neighbors.origin')}
                      labelHidden
                      bind:value={editOrigin}
                      disabled={updateMutationState.isPending}
                    />

                    <div class="flex justify-end gap-2">
                      <Button size="sm" variant="secondary" onclick={cancelEdit}>
                        {m('admin.neighbors.cancel')}
                      </Button>
                      <Button
                        type="submit"
                        size="sm"
                        loading={updateMutationState.isPending}
                        disabled={!normalizedEditOrigin || normalizedEditOrigin === neighbor.origin}
                      >
                        {m('admin.neighbors.save')}
                      </Button>
                    </div>
                  </form>
                {:else}
                  <div class="flex justify-end gap-2">
                    <Button size="sm" variant="secondary" onclick={() => startEdit(neighbor)}>
                      <span aria-hidden="true" class="iconify icon-[uil--edit]"></span>
                      {m('admin.neighbors.edit')}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onclick={() => (deleteTarget = { ...mutationVariables(), neighbor })}
                    >
                      <span aria-hidden="true" class="iconify icon-[uil--trash-alt]"></span>
                      {m('admin.neighbors.delete')}
                    </Button>
                  </div>
                {/if}
              {/snippet}
              <ServerProfileCard
                origin={neighbor.origin}
                imageOrigin={serverOrigin}
                profile={neighborProfile(neighbor.origin)}
                {actions}
                testId="neighbor-card"
              />
            {/each}
          </div>
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
