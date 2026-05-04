<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import { TextInput, TextArea, Button, FormError, z, validate } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const MAX_SPACE_NAME_LENGTH = 42;
  const spaceNameSchema = z
    .string()
    .min(1, 'Space name is required')
    .max(MAX_SPACE_NAME_LENGTH, `Space name cannot exceed ${MAX_SPACE_NAME_LENGTH} characters`);

  // Instances where the user is authenticated AND has permission to create spaces.
  // Permissions are loaded by InstanceSpaceSection; if not yet loaded, include the
  // instance (server enforces the check on the mutation anyway).
  let eligibleInstances = $derived(
    instanceRegistry.instances.filter((i) => {
      const isOrigin = instanceRegistry.isOriginInstance(i.id);
      const isAuthenticated = isOrigin
        ? !!instanceRegistry.getStore(i.id).currentUser.user
        : !!i.token;
      if (!isAuthenticated) return false;

      const perms = instanceRegistry.getStore(i.id).permissions;
      return !perms.loaded || perms.canCreateSpace;
    })
  );

  // Form state
  // svelte-ignore state_referenced_locally
  let selectedInstanceId = $state(eligibleInstances[0]?.id ?? '');
  let name = $state('');
  let description = $state('');
  let isLoading = $state(false);
  let error = $state('');

  // Validation
  let nameError = $derived(name ? validate(spaceNameSchema, name.trim()) : undefined);
  let isValid = $derived(!nameError && name.trim().length > 0 && !!selectedInstanceId);

  $effect(() => {
    if (name || description) {
      error = '';
    }
  });

  // Keep selectedInstanceId valid if instances change
  $effect(() => {
    if (!eligibleInstances.find((i) => i.id === selectedInstanceId)) {
      selectedInstanceId = eligibleInstances[0]?.id ?? '';
    }
  });

  function getInstanceHostname(url: string): string {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();

    if (!isValid) return;

    isLoading = true;
    error = '';

    try {
      const client = graphqlClientManager.getClient(selectedInstanceId).client;
      const result = await client
        .mutation(
          graphql(`
            mutation CreateSpacePage($input: CreateSpaceInput!) {
              createSpace(input: $input) {
                id
                name
                description
              }
            }
          `),
          { input: { name: name.trim(), description: description.trim() || undefined } }
        )
        .toPromise();

      if (result.error) {
        error = result.error.message;
        isLoading = false;
        return;
      }

      // Refresh primarySpaceId on the target instance — see ADR-027 note in
      // the per-instance create page for why.
      instanceRegistry.tryGetStore(selectedInstanceId)?.instance.init();

      isLoading = false;
      goto(resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(selectedInstanceId) }));
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create space';
      isLoading = false;
    }
  }
</script>

<PageTitle title="Create Space" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  {#if eligibleInstances.length === 0}
    <PaneHeader title="Create a New Space" showMobileNav />
    <div class="flex flex-1 flex-col items-center justify-center p-6">
      <div class="text-center">
        <p class="text-lg text-muted">No instances connected</p>
        <p class="mt-2 text-sm text-muted">Connect to an instance first to create a space.</p>
      </div>
    </div>
  {:else}
    <PaneHeader
      title="Create a New Space"
      showMobileNav
      subtitle="Spaces are communities for your friends or your team."
    />

    <div class="flex flex-col gap-6 overflow-y-auto p-6">
      <Panel title="Space Details" icon="iconify uil--edit">
        <form onsubmit={handleSubmit} class="flex flex-col gap-4">
          {#if eligibleInstances.length > 1}
            <div class="flex flex-col gap-1">
              <label for="instance-select" class="text-sm font-medium">Instance</label>
              <select
                id="instance-select"
                bind:value={selectedInstanceId}
                disabled={isLoading}
                class="cursor-pointer rounded-lg border border-border bg-surface-100 px-3 py-2 text-sm text-text focus:border-primary focus:outline-none"
              >
                {#each eligibleInstances as instance (instance.id)}
                  {@const store = instanceRegistry.getStore(instance.id)}
                  <option value={instance.id}>
                    {store.instance.name || getInstanceHostname(instance.url)} ({getInstanceHostname(instance.url)})
                  </option>
                {/each}
              </select>
            </div>
          {/if}

          <TextInput
            id="space-name"
            label="Name"
            bind:value={name}
            placeholder="Enter space name"
            disabled={isLoading}
            error={nameError}
            required
          />

          <TextArea
            id="space-description"
            label="Description"
            bind:value={description}
            placeholder="What's this space about?"
            disabled={isLoading}
            rows={3}
          />

          <FormError {error} />

          <Button type="submit" loading={isLoading} disabled={!isValid} loadingText="Creating...">
            <span class="iconify uil--plus"></span>
            Create Space
          </Button>
        </form>
      </Panel>
    </div>
  {/if}
</div>
