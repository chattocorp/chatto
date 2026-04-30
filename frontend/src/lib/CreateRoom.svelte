<script lang="ts">
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from './gql';
  import { TextInput, TextArea, Button, FormError } from '$lib/ui/form';

  let name = $state('');
  let description = $state('');
  let isLoading = $state(false);
  let error = $state('');

  let {
    spaceId,
    onroomcreated
  }: {
    spaceId: string;
    onroomcreated?: (roomId: string) => void;
  } = $props();

  const connection = useConnection();

  async function handleSubmit(e: Event) {
    e.preventDefault();

    if (!name.trim()) {
      error = 'Room name is required';
      return;
    }

    isLoading = true;
    error = '';

    try {
      const result = await connection().client
        .mutation(
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
              spaceId,
              name: name.trim(),
              description: description.trim() || undefined
            }
          }
        )
        .toPromise();

      if (result.error) {
        error = result.error.message;
        isLoading = false;
        console.error('Error creating room:', result.error);
        return;
      }

      const roomId = result.data!.createRoom.id;

      // Join the newly created room
      const joinResult = await connection().client
        .mutation(
          graphql(`
            mutation JoinRoom($input: JoinRoomInput!) {
              joinRoom(input: $input)
            }
          `),
          { input: { spaceId, roomId } }
        )
        .toPromise();

      if (joinResult.error) {
        error = joinResult.error.message;
        isLoading = false;
        console.error('Error joining room:', joinResult.error);
        return;
      }

      isLoading = false;
      onroomcreated?.(roomId);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create room';
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
    id="room-name"
    label="Room Name"
    bind:value={name}
    placeholder="Enter room name"
    disabled={isLoading}
  />

  <TextArea
    id="room-description"
    label="Description (optional)"
    bind:value={description}
    placeholder="What's this room about?"
    disabled={isLoading}
    rows={3}
  />

  <FormError {error} />

  <Button
    type="submit"
    size="lg"
    loading={isLoading}
    disabled={!name.trim()}
    loadingText="Creating..."
  >
    Create Room
  </Button>
</form>
