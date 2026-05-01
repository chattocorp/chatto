<script lang="ts">
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { graphql } from './gql';
  import { TextInput, TextArea, Button, FormError, z, validate } from '$lib/ui/form';

  const MAX_SPACE_NAME_LENGTH = 42;
  const spaceNameSchema = z
    .string()
    .min(1, 'Space name is required')
    .max(MAX_SPACE_NAME_LENGTH, `Space name cannot exceed ${MAX_SPACE_NAME_LENGTH} characters`);

  let name = $state('');
  let description = $state('');
  let isLoading = $state(false);
  let error = $state('');

  // Validation
  let nameError = $derived(name ? validate(spaceNameSchema, name.trim()) : undefined);
  let isValid = $derived(!nameError && name.trim().length > 0);

  let {
    onspacecreated
  }: {
    onspacecreated?: (spaceId: string) => void;
  } = $props();

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
            mutation CreateSpace($input: CreateSpaceInput!) {
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
        console.error('Error creating space:', result.error);
        return;
      }

      isLoading = false;
      onspacecreated?.(result.data!.createSpace.id);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create space';
      isLoading = false;
    }
  }

  $effect(() => {
    if (name || description) {
      error = '';
    }
  });
</script>

<form onsubmit={handleSubmit} class="space-y-4">
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
    label="Description (optional)"
    bind:value={description}
    placeholder="What's this space about?"
    disabled={isLoading}
    rows={3}
  />

  <FormError {error} />

  <Button type="submit" size="lg" loading={isLoading} disabled={!isValid} loadingText="Creating...">
    <span class="iconify uil--plus"></span>
    Create Space
  </Button>
</form>
