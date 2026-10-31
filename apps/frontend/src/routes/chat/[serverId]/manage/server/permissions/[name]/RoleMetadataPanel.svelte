<!-- @component Role identity, metadata editing, and immediate ping settings. -->
<script lang="ts">
  import type { Role } from '$lib/components/rbac';
  import { untrack } from 'svelte';
  import { Panel } from '$lib/ui';
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

  function saveMetadata(event: SubmitEvent): void {
    event.preventDefault();
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

<Panel title={m('admin.common.role_details')} icon="iconify icon-[uil--shield]">
  <form class="@container/role flex min-w-0 flex-col gap-6" onsubmit={saveMetadata}>
    {#if role.isSystem}
      <div class="grid items-center gap-6 py-2 @min-[38rem]/role:grid-cols-[minmax(0,1fr)_14rem]">
        <div class="flex min-w-0 items-center gap-4 @min-[24rem]/role:gap-5">
          <div
            aria-hidden="true"
            class="relative flex size-14 shrink-0 items-center justify-center rounded-full bg-action/10 text-action @min-[24rem]/role:size-20"
          >
            <span
              aria-hidden="true"
              class={[
                'iconify text-3xl @min-[24rem]/role:text-4xl',
                role.name === 'everyone' ? 'icon-[uil--users-alt]' : 'icon-[uil--shield-check]'
              ]}
            ></span>
            <span
              class="absolute end-0 bottom-0 flex size-7 items-center justify-center rounded-full bg-background text-muted"
            >
              <span aria-hidden="true" class="iconify icon-[uil--lock-alt] text-base"></span>
            </span>
          </div>
          <div class="min-w-0 space-y-2">
            <h3
              class="text-2xl font-semibold tracking-tight break-words text-text-top @min-[24rem]/role:text-3xl"
            >
              {role.displayName}
            </h3>
            {#if role.description}
              <p class="break-words text-muted">{role.description}</p>
            {/if}
          </div>
        </div>
        <div class="space-y-3">
          <dl class="space-y-1">
            <dt class="text-sm text-muted">{m('rbac.role_form.name')}</dt>
            <dd><code class="break-all">{role.name}</code></dd>
          </dl>
          <p class="text-sm text-muted">{m('admin.permissions.system_metadata_locked')}</p>
        </div>
      </div>
    {:else}
      <div class="space-y-2">
        <dl class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <dt class="text-sm text-muted">{m('rbac.role_form.name')}</dt>
          <dd><code class="text-sm break-all">{role.name}</code></dd>
        </dl>
        <p class="flex items-start gap-2 text-sm text-muted">
          <span aria-hidden="true" class="iconify mt-0.5 icon-[uil--lock-alt] shrink-0"></span>
          {m('rbac.role_form.name_locked')}
        </p>
      </div>
    {/if}

    {#if !role.isSystem}
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

    {#if canEditPingable}
      <Checkbox
        id="pingable"
        bind:checked={editPingable}
        label={m('rbac.role_form.pingable')}
        onchange={savePingable}
        disabled={saving || savingPingable}
        description={m('rbac.role_form.pingable_description')}
      />
    {:else}
      <div
        class="flex flex-col items-start gap-3 surface-box p-4 @min-[24rem]/role:flex-row @min-[24rem]/role:items-center @min-[24rem]/role:gap-5"
      >
        <span aria-hidden="true" class="shrink-0 text-2xl font-semibold tracking-tight text-action"
          >@all</span
        >
        <p class="max-w-xl text-muted">{m('admin.permissions.everyone_pingable_description')}</p>
      </div>
    {/if}

    {#if !role.isSystem}
      <div class="flex gap-2">
        <Button type="submit" disabled={!metadataChanged || saving || savingPingable}>
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
  </form>
</Panel>
