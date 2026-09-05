<script lang="ts">
  import { DraftField } from '$lib/components/settings/DraftField.svelte';
  import type { AdminRoomGroup } from '$lib/api-client/adminRoomLayout';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import { buildRoomGroupSettingsUpdate } from './roomGroupSettings';
  import { m } from '$lib/i18n/messages';

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
  const name = new DraftField(() => group.name);
  const description = new DraftField(() => group.description ?? '');

  const changed = $derived(
    name.value.trim() !== name.original || description.value.trim() !== description.original
  );

  function save(event: SubmitEvent): void {
    event.preventDefault();
    if (saving || !name.value.trim() || !changed) return;
    onSave(
      buildRoomGroupSettingsUpdate(
        group.id,
        { name: name.value, description: description.value },
        { name: name.original, description: description.original }
      )
    );
  }
</script>

<Panel title={m('admin.nav.general')} icon="iconify icon-[uil--setting]">
  <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
    <TextInput
      id="room-group-settings-name"
      label={m('admin.rooms_admin.group_name')}
      bind:value={name.value}
      required
      maxlength={80}
      disabled={saving}
    />
    <TextArea
      id="room-group-settings-description"
      label={m('rbac.role_form.description')}
      bind:value={description.value}
      rows={3}
      maxlength={500}
      disabled={saving}
    />
    <div class="flex justify-end">
      <Button type="submit" loading={saving} disabled={!name.value.trim() || !changed}>
        {m('admin.permissions.save_changes')}
      </Button>
    </div>
  </form>
</Panel>
