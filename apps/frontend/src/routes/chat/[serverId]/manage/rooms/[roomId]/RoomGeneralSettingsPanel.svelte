<script lang="ts">
  import { untrack } from 'svelte';
  import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { Panel } from '$lib/components/admin';
  import { Button, Checkbox, TextArea, TextInput } from '$lib/ui/form';
  import {
    hasValidRoomNameCharacters,
    normalizeRoomName,
    ROOM_NAME_MAX_LENGTH,
    roomNameCharacterCount
  } from '$lib/utils/roomName';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import { buildRoomSettingsUpdate } from './roomSettings';
  import * as m from '$lib/i18n/messages';

  let {
    room,
    saving,
    onSave
  }: {
    room: AdminManagedRoom;
    saving: boolean;
    onSave: (update: ReturnType<typeof buildRoomSettingsUpdate>) => void;
  } = $props();

  // The parent keys this editor by room identity and successful save revision.
  const originalName = untrack(() => room.name);
  const originalDescription = untrack(() => room.description ?? '');
  const originalUniversal = untrack(() => room.isUniversal);
  let name = $state(originalName);
  let description = $state(originalDescription);
  let universal = $state(originalUniversal);

  const normalizedName = $derived(normalizeRoomName(name));
  const nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return m['admin.rooms_admin.room_name_empty']();
    if (name !== name.trim()) return m['admin.rooms_admin.room_name_trim']();
    if (!hasValidRoomNameCharacters(normalizedName)) {
      return m['admin.rooms_admin.room_name_charset']();
    }
    if (roomNameCharacterCount(normalizedName) > ROOM_NAME_MAX_LENGTH) {
      return m['admin.rooms_admin.room_name_too_long']();
    }
    return undefined;
  });
  const changed = $derived(
    normalizedName !== originalName ||
      description.trim() !== originalDescription ||
      universal !== originalUniversal
  );

  function save(event: SubmitEvent): void {
    event.preventDefault();
    if (saving || nameError || !name.trim() || !changed) return;
    onSave(
      buildRoomSettingsUpdate(
        room.id,
        { name, description, universal },
        {
          name: originalName,
          description: originalDescription,
          universal: originalUniversal
        }
      )
    );
  }
</script>

<Panel title={m['admin.nav.general']()} icon="iconify uil--setting">
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <TextInput
      id="room-settings-name"
      label={m['rbac.role_form.name']()}
      bind:value={name}
      required
      disabled={saving}
      error={nameError}
    />
    <TextArea
      id="room-settings-description"
      label={m['rbac.role_form.description']()}
      bind:value={description}
      rows={3}
      disabled={saving}
      placeholder={m['admin.rooms_admin.room_description_placeholder']()}
    />
    <Checkbox
      id="room-settings-universal"
      bind:checked={universal}
      disabled={saving}
      label={m['admin.rooms_admin.universal_room']()}
      description={UNIVERSAL_ROOM_HELP_TEXT}
    />
    <div class="flex justify-end">
      <Button type="submit" loading={saving} disabled={!name.trim() || !!nameError || !changed}>
        {m['admin.permissions.save_changes']()}
      </Button>
    </div>
  </form>
</Panel>
