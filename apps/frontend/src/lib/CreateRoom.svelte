<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { normalizeRoomName, roomNameValidationError } from '$lib/utils/roomName';
  import {
    TextInput,
    TextArea,
    Checkbox,
    Button,
    FormError,
    createFormState,
    z
  } from '$lib/ui/form';
  import { ChoiceRow } from '$lib/ui';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    groupId,
    onroomcreated
  }: {
    /** The room group the new channel room is placed into. */
    groupId?: string;
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
    name: roomNameSchema,
    description: z.string(),
    isUniversal: z.boolean(),
    threadingMode: z.string()
  });

  const form = createFormState(schema, {
    name: '',
    description: '',
    isUniversal: false,
    threadingMode: String(RoomThreadingMode.ENABLED)
  });

  const threadingModeOptions = $derived([
    {
      value: String(RoomThreadingMode.REQUIRED),
      label: m('admin.rooms_admin.threading_mode_required'),
      description: m('admin.rooms_admin.threading_mode_required_description')
    },
    {
      value: String(RoomThreadingMode.ENCOURAGED),
      label: m('admin.rooms_admin.threading_mode_encouraged'),
      description: m('admin.rooms_admin.threading_mode_encouraged_description')
    },
    {
      value: String(RoomThreadingMode.ENABLED),
      label: m('admin.rooms_admin.threading_mode_enabled'),
      description: m('admin.rooms_admin.threading_mode_enabled_description')
    },
    {
      value: String(RoomThreadingMode.DISABLED),
      label: m('admin.rooms_admin.threading_mode_disabled'),
      description: m('admin.rooms_admin.threading_mode_disabled_description')
    }
  ]);

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
        description: values.description.trim() || null,
        groupId: targetGroupId,
        universal: values.isUniversal,
        threadingMode: Number(values.threadingMode) as RoomThreadingMode
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

<form onsubmit={handleSubmit} class="space-y-4">
  <TextInput
    id="room-name"
    label={m('room.create.name_label')}
    bind:value={form.values.name}
    error={form.fieldError('name')}
    oninput={handleNameInput}
    placeholder={m('room.create.name_placeholder')}
    disabled={isLoading}
  />

  <TextArea
    id="room-description"
    label={m('room.create.description_label')}
    bind:value={form.values.description}
    placeholder={m('room.create.description_placeholder')}
    disabled={isLoading}
    oninput={clearSubmitError}
    rows={3}
  />

  <Checkbox
    id="room-universal"
    bind:checked={form.values.isUniversal}
    disabled={isLoading}
    onchange={clearSubmitError}
    label={m('room.create.universal_label')}
    description={m('room.create.universal_description')}
  />

  <div class="flex flex-col gap-2">
    <div>
      <p id="room-threading-mode-label" class="font-medium">
        {m('admin.rooms_admin.threading_mode')}
      </p>
    </div>
    <div class="flex flex-col gap-2" role="radiogroup" aria-labelledby="room-threading-mode-label">
      {#each threadingModeOptions as option (option.value)}
        <ChoiceRow
          label={option.label}
          description={option.description}
          selected={form.values.threadingMode === option.value}
          disabled={isLoading}
          onclick={() => {
            form.values.threadingMode = option.value;
            clearSubmitError();
          }}
        />
      {/each}
    </div>
  </div>

  <FormError error={submitError} />

  <Button
    type="submit"
    size="lg"
    loading={isLoading}
    disabled={!form.isValid}
    loadingText={m('room.create.creating')}
  >
    <span class="iconify icon-[uil--plus]"></span>
    {m('room.create.submit')}
  </Button>
</form>
