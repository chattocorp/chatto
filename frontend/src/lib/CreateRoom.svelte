<script lang="ts">
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import { isUnsupportedGraphQLInputFieldError } from '$lib/gql/compatibility';
  import { graphql } from './gql';
  import {
    TextInput,
    TextArea,
    Checkbox,
    Button,
    FormError,
    createFormState,
    z
  } from '$lib/ui/form';

  let {
    groupId,
    onroomcreated
  }: {
    /** The room group the new channel room is placed into. */
    groupId?: string;
    onroomcreated?: (roomId: string) => void;
  } = $props();

  const connection = useConnection();

  const schema = z.object({
    name: z.string().trim().min(1, 'Room name is required'),
    description: z.string(),
    isUniversal: z.boolean()
  });

  const form = createFormState(schema, { name: '', description: '', isUniversal: false });

  const CreateRoomMutation = graphql(`
    mutation CreateRoom($input: CreateRoomInput!) {
      createRoom(input: $input) {
        id
        name
        description
      }
    }
  `);

  const JoinRoomMutation = graphql(`
    mutation JoinRoom($input: JoinRoomInput!) {
      joinRoom(input: $input) {
        id
      }
    }
  `);

  let isLoading = $state(false);
  /** Server-side / network error from the mutations. Validation errors live on form. */
  let submitError = $state('');

  function clearSubmitError() {
    submitError = '';
  }

  const handleSubmit = form.handleSubmit(async (values) => {
    isLoading = true;
    submitError = '';

    try {
      const targetGroupId = groupId;
      if (!targetGroupId) {
        submitError = 'Choose a room group before creating a room.';
        return;
      }

      const input = {
        name: values.name.trim(),
        description: values.description.trim() || undefined,
        groupId: targetGroupId
      };
      const client = connection().client;
      let result = await client
        .mutation(CreateRoomMutation, {
          input: values.isUniversal ? { ...input, isUniversal: true } : input
        })
        .toPromise();

      if (
        values.isUniversal &&
        result.error &&
        isUnsupportedGraphQLInputFieldError(result.error, 'isUniversal')
      ) {
        result = await client.mutation(CreateRoomMutation, { input }).toPromise();
      }

      if (result.error) {
        submitError = result.error.message;
        console.error('Error creating room:', result.error);
        return;
      }

      const roomId = result.data!.createRoom.id;

      const joinResult = await client
        .mutation(JoinRoomMutation, { input: { roomId } })
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
    oninput={clearSubmitError}
    placeholder="Enter room name"
    disabled={isLoading}
  />

  <TextArea
    id="room-description"
    label="Description (optional)"
    bind:value={form.values.description}
    placeholder="What's this room about?"
    disabled={isLoading}
    oninput={clearSubmitError}
    rows={3}
  />

  <Checkbox
    id="room-universal"
    bind:checked={form.values.isUniversal}
    disabled={isLoading}
    onchange={clearSubmitError}
    label="Universal room"
    description={UNIVERSAL_ROOM_HELP_TEXT}
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
