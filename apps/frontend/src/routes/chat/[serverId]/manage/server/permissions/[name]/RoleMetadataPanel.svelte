<script lang="ts">
  import type { Role } from '$lib/components/rbac';
  import { untrack } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, Checkbox, TextArea, TextInput } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';

  let {
    role,
    saving,
    savingPingable,
    onSaveMetadata,
    onSavePingable,
    onDelete
  }: {
    role: Role;
    saving: boolean;
    savingPingable: boolean;
    onSaveMetadata: (displayName: string, description: string) => void;
    onSavePingable: (pingable: boolean) => Promise<boolean>;
    onDelete: () => void;
  } = $props();

  // The parent keys this editor by role and successful metadata revision.
  let editDisplayName = $state(untrack(() => role.displayName));
  let editDescription = $state(untrack(() => role.description));
  let editPingable = $state(untrack(() => role.pingable));

  const metadataChanged = $derived(
    editDisplayName !== role.displayName || editDescription !== role.description
  );
  const canEditPingable = $derived(role.name !== 'everyone');

  function saveMetadata(): void {
    if (!metadataChanged || saving || savingPingable) return;
    onSaveMetadata(editDisplayName, editDescription);
  }

  async function savePingable(event: Event): Promise<void> {
    if (!canEditPingable || saving || savingPingable) return;
    const target = event.currentTarget as HTMLInputElement;
    const nextPingable = target.checked;
    const previousPingable = role.pingable;
    if (nextPingable === previousPingable) return;
    if (!(await onSavePingable(nextPingable))) editPingable = previousPingable;
  }
</script>

<Panel title={m('admin.common.role_details')} icon="iconify icon-[uil--info-circle]">
  <div class="flex flex-col gap-4">
    <div>
      <div class="mb-1 text-sm font-medium">{m('rbac.role_form.name')}</div>
      <code class="rounded bg-surface-emphasized px-2 py-1">{role.name}</code>
      <p class="mt-1 text-xs text-muted">{m('rbac.role_form.name_locked')}</p>
    </div>

    {#if role.isSystem}
      <div>
        <div class="mb-1 text-sm font-medium">{m('rbac.role_form.display_name')}</div>
        <div class="text-text">{role.displayName}</div>
      </div>
      <div>
        <div class="mb-1 text-sm font-medium">{m('rbac.role_form.description')}</div>
        <div class="text-muted">{role.description}</div>
      </div>
      <p class="text-sm text-muted">{m('admin.permissions.system_metadata_locked')}</p>
    {:else}
      <TextInput
        id="displayName"
        testid="role-form-display-name"
        label={m('rbac.role_form.display_name')}
        bind:value={editDisplayName}
      />
      <TextArea
        id="description"
        testid="role-form-description"
        label={m('rbac.role_form.description')}
        bind:value={editDescription}
      />
    {/if}

    <Checkbox
      id="pingable"
      bind:checked={editPingable}
      label={m('rbac.role_form.pingable')}
      onchange={savePingable}
      disabled={saving || savingPingable || !canEditPingable}
      description={canEditPingable
        ? m('rbac.role_form.pingable_description')
        : m('admin.permissions.everyone_pingable_description')}
    />

    {#if !role.isSystem}
      <div class="flex gap-2">
        <Button
          variant="neutral"
          disabled={!metadataChanged || saving || savingPingable}
          onclick={saveMetadata}
        >
          {saving ? m('rbac.role_form.saving') : m('admin.permissions.save_changes')}
        </Button>
      </div>

      <div class="mt-4 border-t border-border pt-4">
        <div class="mb-2 text-sm font-medium text-danger">
          {m('admin.common.danger_zone')}
        </div>
        <p class="mb-3 text-sm text-muted">
          {m('admin.permissions.delete_role_description')}
        </p>
        <Button variant="danger" onclick={onDelete}>
          {m('rbac.delete_role.action')}
        </Button>
      </div>
    {/if}
  </div>
</Panel>
