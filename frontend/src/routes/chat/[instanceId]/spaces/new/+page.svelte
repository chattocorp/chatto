<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';

  const getInstanceId = getActiveInstance();
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

  const instancePerms = getInstancePermissions();
  let canCreateSpace = $derived(
    !instancePerms.current.loaded ? true : instancePerms.current.canCreateSpace
  );

  // Form state
  let name = $state('');
  let description = $state('');
  let isLoading = $state(false);
  let error = $state('');

  // Validation
  let nameError = $derived(name ? validate(spaceNameSchema, name.trim()) : undefined);
  let isValid = $derived(!nameError && name.trim().length > 0);

  $effect(() => {
    if (name || description) {
      error = '';
    }
  });

  async function handleSubmit(e: Event) {
    e.preventDefault();

    if (!isValid) {
      return;
    }

    isLoading = true;
    error = '';

    try {
      const result = await graphqlClientManager.originClient.client
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

      // Refresh primarySpaceId in case the new space is now the primary one
      // (auto-derived single-space case post-ADR-027). Without this, the
      // chat-root page would still see the cached empty primarySpaceId and
      // fall through to the empty-state instead of redirecting to the new
      // space's first room.
      instanceRegistry.tryGetStore(getInstanceId())?.instance.init();

      isLoading = false;
      goto(resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(getInstanceId()) }));
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create space';
      isLoading = false;
    }
  }
</script>

<PageTitle title="Create Space" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  {#if !canCreateSpace}
    <PaneHeader title="Create a New Space" showMobileNav />
    <div class="flex flex-1 flex-col items-center justify-center p-6">
      <div class="text-center">
        <p class="text-lg text-muted">Access Denied</p>
        <p class="mt-2 text-sm text-muted">You do not have permission to create spaces.</p>
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
