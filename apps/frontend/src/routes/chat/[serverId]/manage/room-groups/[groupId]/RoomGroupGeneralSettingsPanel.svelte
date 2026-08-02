<script lang="ts">
  import { untrack } from 'svelte';
  import type { AdminRoomGroup } from '$lib/api-client/adminRoomLayout';
  import { Panel } from '$lib/components/admin';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import { buildRoomGroupSettingsUpdate } from './roomGroupSettings';
  import * as m from '$lib/i18n/messages';

  let {
    group,
    saving,
    onSave
  }: {
    group: AdminRoomGroup;
    saving: boolean;
    onSave: (update: ReturnType<typeof buildRoomGroupSettingsUpdate>) => void;
  } = $props();

  // The parent keys this editor by group identity and successful save revision.
  const originalName = untrack(() => group.name);
  const originalDescription = untrack(() => group.description ?? '');
  let name = $state(originalName);
  let description = $state(originalDescription);
  const changed = $derived(
    name.trim() !== originalName || description.trim() !== originalDescription
  );

  function save(event: SubmitEvent): void {
    event.preventDefault();
    if (saving || !name.trim() || !changed) return;
    onSave(
      buildRoomGroupSettingsUpdate(
        group.id,
        { name, description },
        { name: originalName, description: originalDescription }
      )
    );
  }
</script>

<Panel title={m['admin.nav.general']()} icon="iconify uil--setting">
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <TextInput
      id="room-group-settings-name"
      label={m['admin.rooms_admin.group_name']()}
      bind:value={name}
      required
      maxlength={80}
      disabled={saving}
    />
    <TextArea
      id="room-group-settings-description"
      label={m['rbac.role_form.description']()}
      bind:value={description}
      rows={3}
      maxlength={500}
      disabled={saving}
    />
    <div class="flex justify-end">
      <Button type="submit" loading={saving} disabled={!name.trim() || !changed}>
        {m['admin.permissions.save_changes']()}
      </Button>
    </div>
  </form>
</Panel>
