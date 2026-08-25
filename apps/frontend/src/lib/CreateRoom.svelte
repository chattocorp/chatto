<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { normalizeRoomName, roomNameValidationError } from '$lib/utils/roomName';
  import { TextInput, createFormState, z } from '$lib/ui/form';
  import { FormDialog } from '$lib/ui';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    groupId,
    visible = $bindable(true),
    onclose,
    onroomcreated
  }: {
    /** The room group the new channel room is placed into. */
    groupId?: string;
    visible?: boolean;
    onclose?: () => void;
    onroomcreated?: (roomId: string) => void;
  } = $props();

  const serverScope = useServerScope();

  const roomNameSchema = z
    .string()
    .refine((name) => roomNameValidationError(name) !== 'empty', m('room.create.name_required'))
    .refine(
      (name) => roomNameValidationError(name) !== 'too_long',
      m('admin.rooms_admin.room_name_too_long')
    )
    .refine(
      (name) => roomNameValidationError(name) !== 'invalid',
      m('admin.rooms_admin.room_name_invalid')
    );

  const schema = z.object({
    name: roomNameSchema
  });

  const form = createFormState(schema, {
    name: ''
  });

  let isLoading = $state(false);
  /** Server-side / network error from the mutations. Validation errors live on form. */
  let submitError = $state('');

  function clearSubmitError() {
    submitError = '';
  }

  function handleNameInput() {
    form.touch('name');
    clearSubmitError();
  }

  function handleClose() {
    visible = false;
    onclose?.();
  }

  const handleSubmit = form.handleSubmit(async (values) => {
    isLoading = true;
    submitError = '';

    try {
      const targetGroupId = groupId;
      if (!targetGroupId) {
        submitError = m('room.create.missing_group');
        return;
      }

      const api = serverScope.connection.getAPI(createRoomCommandAPI);
      const created = await api.createRoom({
        name: normalizeRoomName(values.name),
        description: null,
        groupId: targetGroupId,
        universal: false,
        threadingMode: RoomThreadingMode.ENABLED
      });
      const roomId = created?.id;
      if (!roomId) throw new Error(m('room.create.failed'));

      await api.joinRoom(roomId);

      if (!serverScope.isCurrent()) return;
      onroomcreated?.(roomId);
    } catch (err) {
      submitError = err instanceof Error ? err.message : m('room.create.failed');
    } finally {
      isLoading = false;
    }
  });
</script>

<FormDialog
  bind:visible
  title={m('admin.rooms_admin.create_room')}
  size="sm"
  submitLabel={m('room.create.submit_and_configure')}
  submitIcon="iconify icon-[uil--plus]"
  submitLoadingText={m('room.create.creating')}
  loading={isLoading}
  disabled={!form.isValid}
  error={submitError}
  onsubmit={handleSubmit}
  onclose={handleClose}
>
  <TextInput
    id="room-name"
    label={m('room.create.name_label')}
    bind:value={form.values.name}
    error={form.fieldError('name')}
    oninput={handleNameInput}
    placeholder={m('room.create.name_placeholder')}
    disabled={isLoading}
  />
</FormDialog>
