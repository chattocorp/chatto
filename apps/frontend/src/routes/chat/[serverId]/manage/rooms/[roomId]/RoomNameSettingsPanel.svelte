<!--
@component

Name and description of a room. The fields change together, so they submit as
one form with Save and Discard actions. A successful save resets only this form.
-->
<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { Panel } from '$lib/ui';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import { normalizeRoomName, roomNameValidationError } from '@chatto/client/util/roomName';
  import { m } from '$lib/i18n/messages';
  import { buildRoomDetailsPatch, type RoomSettingsPatch } from './roomSettings';

  let {
    room,
    onSave
  }: {
    room: AdminManagedRoom;
    /** Saves a sparse patch. Resolves to true when the room accepted it. */
    onSave: (patch: RoomSettingsPatch) => Promise<boolean>;
  } = $props();

  const name = new DraftField(() => room.name);
  const description = new DraftField(() => room.description ?? '');
  let saving = $state(false);

  const normalizedName = $derived(normalizeRoomName(name.value));
  const trimmedDescription = $derived(description.value.trim());
  const nameError = $derived.by(() => {
    if (!name.value) return undefined;
    if (name.value.trim() === '') return m('admin.rooms_admin.room_name_empty');
    if (name.value !== name.value.trim()) return m('admin.rooms_admin.room_name_trim');
    const validationError = roomNameValidationError(normalizedName);
    if (validationError === 'empty') return m('admin.rooms_admin.room_name_empty');
    if (validationError === 'too_long') {
      return m('admin.rooms_admin.room_name_too_long');
    }
    if (validationError === 'invalid') return m('admin.rooms_admin.room_name_invalid');
    return undefined;
  });
  const changed = $derived(
    normalizedName !== name.original || trimmedDescription !== description.original
  );

  /** Includes unnormalised input so navigation and Discard also protect invalid drafts. */
  const edited = $derived(
    name.value !== name.original || description.value !== description.original
  );

  /** Lets the page guard navigation without copying the section's draft state. */
  export function hasChanges(): boolean {
    return edited || saving;
  }

  function discard(): void {
    name.value = room.name;
    description.value = room.description ?? '';
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (saving || nameError || !name.value.trim() || !changed) return;
    const patch = buildRoomDetailsPatch(
      { name: name.value, description: description.value },
      { name: name.original, description: description.original }
    );
    saving = true;
    try {
      if (await onSave(patch)) discard();
    } finally {
      saving = false;
    }
  }
</script>

<Panel
  title={m('admin.rooms_admin.settings.name_title')}
  subtitle={m('admin.rooms_admin.settings.name_subtitle')}
  icon="iconify icon-[uil--edit]"
>
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <TextInput
      id="room-settings-name"
      label={m('rbac.role_form.name')}
      bind:value={name.value}
      required
      disabled={saving}
      error={nameError}
    />
    <TextArea
      id="room-settings-description"
      label={m('rbac.role_form.description')}
      bind:value={description.value}
      rows={3}
      disabled={saving}
      placeholder={m('admin.rooms_admin.room_description_placeholder')}
    />
    <div class="flex justify-end gap-2">
      <Button variant="secondary" disabled={saving || !edited} onclick={discard}>
        {m('admin.rooms_admin.settings.discard_changes')}
      </Button>
      <Button
        type="submit"
        loading={saving}
        disabled={!name.value.trim() || !!nameError || !changed}
      >
        {m('admin.permissions.save_changes')}
      </Button>
    </div>
  </form>
</Panel>
