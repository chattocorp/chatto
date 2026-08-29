<script lang="ts">
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { createNeighborAPI, type Neighbor } from '$lib/api-client/neighbors';
  import ServerProfileCard from '$lib/components/ServerProfileCard.svelte';
  import TestimonialText from '$lib/components/TestimonialText.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { loadServerProfiles, serverOriginFromInput } from '$lib/serverDirectory';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { m } from '$lib/i18n/messages';
  import { ConfirmDialog, EmptyState, Hint, PaneContent } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const serverScope = useServerScope();
  let newOrigin = $state('');
  let newTestimonial = $state('');
  let editOrigin = $state('');
  let editTestimonial = $state('');
  const normalizedNewOrigin = $derived(serverOriginFromInput(newOrigin));
  const normalizedEditOrigin = $derived(serverOriginFromInput(editOrigin));

  type NeighborMutationVariables = {
    serverId: string;
    connection: ServerConnection;
    queryKey: ReturnType<typeof adminQueryKeys.neighbors>;
  };

  type CreateVariables = NeighborMutationVariables & { origin: string; testimonial: string };
  type UpdateVariables = NeighborMutationVariables & {
    neighbor: Neighbor;
    origin: string;
    testimonial: string;
  };
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
      mutationFn: ({ connection, origin, testimonial }: CreateVariables) =>
        connection.getAPI(createNeighborAPI).create(origin, testimonial),
      onSuccess: (neighbor, variables) => {
        if (!isCurrent(variables)) return;
        queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) => [
          ...current,
          neighbor
        ]);
        newOrigin = '';
        newTestimonial = '';
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
      mutationFn: ({ connection, neighbor, origin, testimonial }: UpdateVariables) =>
        connection.getAPI(createNeighborAPI).update(neighbor, origin, testimonial),
      onSuccess: (updated, variables) => {
        if (!isCurrent(variables)) return;
        queryClient.setQueryData<Neighbor[]>(variables.queryKey, (current = []) =>
          current.map((neighbor) => (neighbor.id === updated.id ? updated : neighbor))
        );
        editTarget = null;
        editOrigin = '';
        editTestimonial = '';
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
  const profilesQuery = createQuery(
    () => ({
      queryKey: ['public', 'neighbor-profiles', neighbors.map((neighbor) => neighbor.origin)],
      queryFn: ({ signal }) =>
        loadServerProfiles(
          neighbors.map((neighbor) => neighbor.origin),
          { signal }
        ),
      enabled: neighbors.length > 0
    }),
    () => queryClient
  );
  const profilesByOrigin = $derived(
    new Map((profilesQuery.data ?? []).map((entry) => [entry.origin, entry.profile]))
  );

  function startEdit(neighbor: Neighbor) {
    editTarget = {
      ...mutationVariables(),
      neighbor,
      origin: neighbor.origin,
      testimonial: neighbor.testimonial ?? ''
    };
    editOrigin = neighbor.origin;
    editTestimonial = neighbor.testimonial ?? '';
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
    editTestimonial = '';
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
          class="flex max-w-3xl flex-col gap-4"
          onsubmit={(event) => {
            event.preventDefault();
            if (normalizedNewOrigin)
              createMutationState.mutate({
                ...mutationVariables(),
                origin: normalizedNewOrigin,
                testimonial: newTestimonial.trim()
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
          <TextArea
            id="new-neighbor-testimonial"
            label={m('admin.neighbors.testimonial')}
            description={m('admin.neighbors.testimonial_help')}
            bind:value={newTestimonial}
            maxlength={500}
            rows={4}
            disabled={createMutationState.isPending}
          />
          <div class="flex justify-end">
            <Button
              type="submit"
              loading={createMutationState.isPending}
              disabled={!normalizedNewOrigin}
            >
              <span class="iconify icon-[uil--plus]"></span>
              {m('admin.neighbors.add')}
            </Button>
          </div>
        </form>
      </Panel>

      <Panel title={m('admin.neighbors.list_title')} count={neighbors.length || undefined}>
        {#if neighborsQuery.error}
          <div class="mb-4"><Hint tone="danger">{String(neighborsQuery.error)}</Hint></div>
        {/if}

        {#if neighborsQuery.isPending && neighbors.length === 0}
          <div class="text-muted">{m('admin.common.loading')}</div>
        {:else if neighbors.length === 0}
          <EmptyState icon="icon-[uil--server-connection]" title={m('admin.neighbors.empty')} />
        {:else}
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {#each neighbors as neighbor (neighbor.id)}
              {#snippet details()}
                {#if editTarget?.neighbor.id !== neighbor.id && neighbor.testimonial}
                  <figure class="surface-box p-3" data-testid="neighbor-testimonial">
                    <blockquote class="text-sm">
                      <TestimonialText testimonial={neighbor.testimonial} />
                    </blockquote>
                    <figcaption class="mt-2 text-sm font-medium text-muted">
                      {m('admin.neighbors.testimonial_label')}
                    </figcaption>
                  </figure>
                {/if}
              {/snippet}
              {#snippet actions()}
                <div class="flex flex-col gap-3">
                  {#if editTarget?.neighbor.id === neighbor.id}
                    <TextInput
                      id={`neighbor-origin-${neighbor.id}`}
                      label={m('admin.neighbors.origin')}
                      labelHidden
                      bind:value={editOrigin}
                      disabled={updateMutationState.isPending}
                    />
                    <TextArea
                      id={`neighbor-testimonial-${neighbor.id}`}
                      label={m('admin.neighbors.testimonial')}
                      description={m('admin.neighbors.testimonial_help')}
                      bind:value={editTestimonial}
                      maxlength={500}
                      rows={4}
                      disabled={updateMutationState.isPending}
                    />
                  {/if}

                  <div class="flex justify-end gap-2">
                    {#if editTarget?.neighbor.id === neighbor.id}
                      <Button size="sm" variant="secondary" onclick={cancelEdit}>
                        {m('admin.neighbors.cancel')}
                      </Button>
                      <Button
                        size="sm"
                        loading={updateMutationState.isPending}
                        disabled={!normalizedEditOrigin ||
                          (normalizedEditOrigin === neighbor.origin &&
                            editTestimonial.trim() === (neighbor.testimonial ?? ''))}
                        onclick={() =>
                          editTarget &&
                          normalizedEditOrigin &&
                          updateMutationState.mutate({
                            ...editTarget,
                            origin: normalizedEditOrigin,
                            testimonial: editTestimonial.trim()
                          })}
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
                </div>
              {/snippet}
              <ServerProfileCard
                origin={neighbor.origin}
                profile={profilesQuery.isPending
                  ? undefined
                  : (profilesByOrigin.get(neighbor.origin) ?? null)}
                {details}
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
