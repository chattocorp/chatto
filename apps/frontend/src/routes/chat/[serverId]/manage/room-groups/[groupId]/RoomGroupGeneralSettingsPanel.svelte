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
  let originalName = $state(untrack(() => group.name));
  let originalDescription = $state(untrack(() => group.description ?? ''));
  let name = $state(untrack(() => group.name));
  let description = $state(untrack(() => group.description ?? ''));

  // Query snapshots may refresh while this editor is mounted. Adopt each remote
  // field only when that field is pristine, preserving unrelated local edits.
  $effect(() => {
    const nextName = group.name;
    const nextDescription = group.description ?? '';
    untrack(() => {
      const nameWasPristine = name === originalName;
      const descriptionWasPristine = description === originalDescription;
      originalName = nextName;
      originalDescription = nextDescription;
      if (nameWasPristine) name = nextName;
      if (descriptionWasPristine) description = nextDescription;
    });
  });
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
