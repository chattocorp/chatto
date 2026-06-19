<script lang="ts">
  import { TextInput, TextArea, Button, FormError, createFormState, z } from '$lib/ui/form';
  import { tryWireCreateRoom, tryWireJoinRoom } from '$lib/wire';

  let {
    groupId,
    onroomcreated
  }: {
    /** The room group the new channel room is placed into. */
    groupId?: string;
    onroomcreated?: (roomId: string) => void;
  } = $props();

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
      const targetGroupId = groupId;
      if (!targetGroupId) {
        submitError = 'Choose a room group before creating a room.';
        return;
      }

      const roomId = await tryWireCreateRoom({
        name: values.name.trim(),
        description: values.description.trim() || undefined,
        groupId: targetGroupId
      });
      if (!roomId) {
        submitError = 'Failed to create room';
        return;
      }

      const handledByWire = await tryWireJoinRoom({ roomId });
      if (!handledByWire) {
        submitError = 'Failed to join room';
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
