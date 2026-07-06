<script lang="ts">
  import { useConnection } from '$lib/state/server/connection.svelte';
  import * as m from '$lib/i18n/messages';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
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
    name: z.string().trim().min(1, m['room.create.name_required']()),
    description: z.string(),
    isUniversal: z.boolean()
  });

  const form = createFormState(schema, { name: '', description: '', isUniversal: false });

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
        submitError = m['room.create.missing_group']();
        return;
      }

      const conn = connection();
      const api = createRoomCommandAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const created = await api.createRoom({
        name: values.name.trim(),
        description: values.description.trim() || null,
        groupId: targetGroupId,
        universal: values.isUniversal
      });
      const roomId = created?.id;
      if (!roomId) throw new Error(m['room.create.failed']());

      await api.joinRoom(roomId);

      onroomcreated?.(roomId);
    } catch (err) {
      submitError = err instanceof Error ? err.message : m['room.create.failed']();
    } finally {
      isLoading = false;
    }
  });
</script>

<form onsubmit={handleSubmit} class="space-y-4">
  <TextInput
    id="room-name"
    label={m['room.create.name_label']()}
    bind:value={form.values.name}
    error={form.fieldError('name')}
    onkeydown={() => form.touch('name')}
    oninput={clearSubmitError}
    placeholder={m['room.create.name_placeholder']()}
    disabled={isLoading}
  />

  <TextArea
    id="room-description"
    label={m['room.create.description_label']()}
    bind:value={form.values.description}
    placeholder={m['room.create.description_placeholder']()}
    disabled={isLoading}
    oninput={clearSubmitError}
    rows={3}
  />

  <Checkbox
    id="room-universal"
    bind:checked={form.values.isUniversal}
    disabled={isLoading}
    onchange={clearSubmitError}
    label={m['room.create.universal_label']()}
    description={m['room.create.universal_description']()}
  />

  <FormError error={submitError} />

  <Button
    type="submit"
    size="lg"
    loading={isLoading}
    disabled={!form.isValid}
    loadingText={m['room.create.creating']()}
  >
    <span class="iconify uil--plus"></span>
    {m['room.create.submit']()}
  </Button>
</form>
