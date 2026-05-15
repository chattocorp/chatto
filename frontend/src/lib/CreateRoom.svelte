<script lang="ts">
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from './gql';
  import { TextInput, TextArea, Button, FormError, createFormState, z } from '$lib/ui/form';

  let {
    groupId,
    onroomcreated
  }: {
    /** When provided, the new room is placed into this set. */
    groupId?: string;
    onroomcreated?: (roomId: string) => void;
  } = $props();

  const connection = useConnection();

  const schema = z.object({
    name: z.string().trim().min(1, 'Room name is required'),
    description: z.string()
  });

  const form = createFormState(schema, { name: '', description: '' });

  let isLoading = $state(false);
  /** Server-side / network error from the mutations. Validation errors live on form. */
  let submitError = $state('');

  // Clear stale submit errors when the user types.
  $effect(() => {
    if (form.values.name || form.values.description) {
      submitError = '';
    }
  });

  const handleSubmit = form.handleSubmit(async (values) => {
    isLoading = true;
    submitError = '';

    try {
      const result = await connection()
        .client.mutation(
          graphql(`
            mutation CreateRoom($input: CreateRoomInput!) {
              createRoom(input: $input) {
                id
                name
                description
              }
            }
          `),
          {
            input: {
              name: values.name.trim(),
              description: values.description.trim() || undefined,
              groupId: groupId ?? undefined
            }
          }
        )
        .toPromise();

      if (result.error) {
        submitError = result.error.message;
        console.error('Error creating room:', result.error);
        return;
      }

      const roomId = result.data!.createRoom.id;

      const joinResult = await connection()
        .client.mutation(
          graphql(`
            mutation JoinRoom($input: JoinRoomInput!) {
              joinRoom(input: $input)
            }
          `),
          { input: { roomId } }
        )
        .toPromise();

      if (joinResult.error) {
        submitError = joinResult.error.message;
        console.error('Error joining room:', joinResult.error);
        return;
      }

      onroomcreated?.(roomId);
    } catch (err) {
      submitError = err instanceof Error ? err.message : 'Failed to create room';
    } finally {
      isLoading = false;
    }
  });
</script>

<form onsubmit={handleSubmit} class="space-y-4">
  <TextInput
    id="room-name"
    label="Room Name"
    bind:value={form.values.name}
    error={form.fieldError('name')}
    onkeydown={() => form.touch('name')}
    placeholder="Enter room name"
    disabled={isLoading}
  />

  <TextArea
    id="room-description"
    label="Description (optional)"
    bind:value={form.values.description}
    placeholder="What's this room about?"
    disabled={isLoading}
    rows={3}
  />

  <FormError error={submitError} />

  <Button
    type="submit"
    size="lg"
    loading={isLoading}
    disabled={!form.isValid}
    loadingText="Creating..."
  >
    <span class="iconify uil--plus"></span>
    Create Room
  </Button>
</form>
